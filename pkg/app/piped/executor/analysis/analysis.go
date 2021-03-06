// Copyright 2020 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	httpprovider "github.com/pipe-cd/pipe/pkg/app/piped/analysisprovider/http"
	"github.com/pipe-cd/pipe/pkg/app/piped/analysisprovider/log"
	"github.com/pipe-cd/pipe/pkg/app/piped/analysisprovider/metrics"
	"github.com/pipe-cd/pipe/pkg/app/piped/executor"
	"github.com/pipe-cd/pipe/pkg/config"
	"github.com/pipe-cd/pipe/pkg/model"
)

type Executor struct {
	executor.Input

	repoDir             string
	config              *config.Config
	startTime           time.Time
	previousElapsedTime time.Duration
}

type registerer interface {
	Register(stage model.Stage, f executor.Factory) error
}

func Register(r registerer) {
	f := func(in executor.Input) executor.Executor {
		return &Executor{
			Input: in,
		}
	}
	r.Register(model.StageAnalysis, f)
}

// templateArgs allows deployment-specific data to be embedded in the analysis template.
// NOTE: Changing its fields will force users to change the template definition.
type templateArgs struct {
	App struct {
		Name string
		Env  string
	}
	K8s struct {
		Namespace string
	}
	// User-defined custom args.
	Args map[string]string
}

// Execute spawns and runs multiple analyzer that run a query at the regular time.
// Any on of those fail then the stage ends with failure.
func (e *Executor) Execute(sig executor.StopSignal) model.StageStatus {
	e.startTime = time.Now()
	ctx := sig.Context()
	options := e.StageConfig.AnalysisStageOptions
	if options == nil {
		e.Logger.Error("missing analysis configuration for ANALYSIS stage")
		return model.StageStatus_STAGE_FAILURE
	}

	ds, err := e.RunningDSP.Get(ctx, e.LogPersister)
	if err != nil {
		e.LogPersister.Errorf("Failed to prepare running deploy source data (%v)", err)
		return model.StageStatus_STAGE_FAILURE
	}
	e.repoDir = ds.RepoDir
	e.config = ds.DeploymentConfig

	templateCfg, ok, err := config.LoadAnalysisTemplate(e.repoDir)
	if err != nil {
		e.LogPersister.Error(err.Error())
		return model.StageStatus_STAGE_FAILURE
	}
	if !ok {
		e.Logger.Info("config file for AnalysisTemplate not found")
		templateCfg = &config.AnalysisTemplateSpec{}
	}

	timeout := time.Duration(options.Duration)
	e.previousElapsedTime = e.retrievePreviousElapsedTime()
	if e.previousElapsedTime > 0 {
		// Restart from the middle.
		timeout -= e.previousElapsedTime
	}
	defer e.saveElapsedTime(ctx)

	ctx, cancel := context.WithTimeout(sig.Context(), timeout)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	// Run analyses with metrics providers.
	mf := metrics.NewFactory(e.Logger)
	for i := range options.Metrics {
		analyzer, err := e.newAnalyzerForMetrics(i, &options.Metrics[i], templateCfg, mf)
		if err != nil {
			e.LogPersister.Error(err.Error())
			continue
		}
		eg.Go(func() error {
			e.LogPersister.Infof("[%s] Start analysis for %s", analyzer.id, analyzer.providerType)
			return analyzer.run(ctx)
		})
	}
	// Run analyses with logging providers.
	lf := log.NewFactory(e.Logger)
	for i := range options.Logs {
		analyzer, err := e.newAnalyzerForLog(i, &options.Logs[i], templateCfg, lf)
		if err != nil {
			e.LogPersister.Error(err.Error())
			continue
		}
		eg.Go(func() error {
			e.LogPersister.Infof("[%s] Start analysis for %s", analyzer.id, analyzer.providerType)
			return analyzer.run(ctx)
		})
	}
	// Run analyses with http providers.
	for i := range options.Https {
		analyzer, err := e.newAnalyzerForHTTP(i, &options.Https[i], templateCfg)
		if err != nil {
			e.LogPersister.Error(err.Error())
			continue
		}
		eg.Go(func() error {
			e.LogPersister.Infof("[%s] Start analysis for %s", analyzer.id, analyzer.providerType)
			return analyzer.run(ctx)
		})
	}

	if err := eg.Wait(); err != nil {
		e.LogPersister.Errorf("Analysis failed: %s", err.Error())
		return model.StageStatus_STAGE_FAILURE
	}

	e.LogPersister.Success("All analyses were successful.")
	return model.StageStatus_STAGE_SUCCESS
}

const elapsedTimeKey = "elapsedTime"

// saveElapsedTime stores the elapsed time of analysis stage into metadata persister.
// The analysis stage can be restarted from the middle even if it ends unexpectedly,
// that's why count should be stored.
func (e *Executor) saveElapsedTime(ctx context.Context) {
	elapsedTime := time.Since(e.startTime) + e.previousElapsedTime
	metadata := map[string]string{
		elapsedTimeKey: elapsedTime.String(),
	}
	if err := e.MetadataStore.SetStageMetadata(ctx, e.Stage.Id, metadata); err != nil {
		e.Logger.Error("failed to store metadata", zap.Error(err))
	}
}

// retrievePreviousElapsedTime sets the elapsed time of analysis stage by decoding metadata.
func (e *Executor) retrievePreviousElapsedTime() time.Duration {
	metadata, ok := e.MetadataStore.GetStageMetadata(e.Stage.Id)
	if !ok {
		return 0
	}
	s, ok := metadata[elapsedTimeKey]
	if !ok {
		return 0
	}
	et, err := time.ParseDuration(s)
	if err != nil {
		e.Logger.Error("unexpected elapsed time is stored", zap.String("stored-value", s), zap.Error(err))
		return 0
	}
	return et
}

func (e *Executor) newAnalyzerForMetrics(i int, templatable *config.TemplatableAnalysisMetrics, templateCfg *config.AnalysisTemplateSpec, factory *metrics.Factory) (*analyzer, error) {
	cfg, err := e.getMetricsConfig(templatable, templateCfg, templatable.Template.Args)
	if err != nil {
		return nil, err
	}
	provider, err := e.newMetricsProvider(cfg.Provider, factory)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("metrics-%d", i)
	return newAnalyzer(id, provider.Type(), func(ctx context.Context) (bool, error) {
		e.LogPersister.Infof("[%s] Run query against %s: %q", id, provider.Type(), cfg.Query)
		return provider.RunQuery(ctx, cfg.Query, cfg.Expected)
	}, time.Duration(cfg.Interval), cfg.FailureLimit, e.Logger, e.LogPersister), nil
}

func (e *Executor) newAnalyzerForLog(i int, templatable *config.TemplatableAnalysisLog, templateCfg *config.AnalysisTemplateSpec, factory *log.Factory) (*analyzer, error) {
	cfg, err := e.getLogConfig(templatable, templateCfg, templatable.Template.Args)
	if err != nil {
		return nil, err
	}
	provider, err := e.newLogProvider(cfg.Provider, factory)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("log-%d", i)
	return newAnalyzer(id, provider.Type(), func(ctx context.Context) (bool, error) {
		e.LogPersister.Infof("[%s] Run query against %s: %q", id, provider.Type(), cfg.Query)
		return provider.RunQuery(ctx, cfg.Query)
	}, time.Duration(cfg.Interval), cfg.FailureLimit, e.Logger, e.LogPersister), nil
}

func (e *Executor) newAnalyzerForHTTP(i int, templatable *config.TemplatableAnalysisHTTP, templateCfg *config.AnalysisTemplateSpec) (*analyzer, error) {
	cfg, err := e.getHTTPConfig(templatable, templateCfg, templatable.Template.Args)
	if err != nil {
		return nil, err
	}
	provider := httpprovider.NewProvider(time.Duration(cfg.Timeout))
	id := fmt.Sprintf("http-%d", i)
	return newAnalyzer(id, provider.Type(), func(ctx context.Context) (bool, error) {
		e.LogPersister.Infof("[%s] Start running query against %s: %s %s", id, provider.Type(), cfg.Method, cfg.URL)
		return provider.Run(ctx, cfg)
	}, time.Duration(cfg.Interval), cfg.FailureLimit, e.Logger, e.LogPersister), nil
}

func (e *Executor) newMetricsProvider(providerName string, factory *metrics.Factory) (metrics.Provider, error) {
	cfg, ok := e.PipedConfig.GetAnalysisProvider(providerName)
	if !ok {
		return nil, fmt.Errorf("unknown provider name %s", providerName)
	}
	provider, err := factory.NewProvider(&cfg)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (e *Executor) newLogProvider(providerName string, factory *log.Factory) (log.Provider, error) {
	cfg, ok := e.PipedConfig.GetAnalysisProvider(providerName)
	if !ok {
		return nil, fmt.Errorf("unknown provider name %s", providerName)
	}
	provider, err := factory.NewProvider(&cfg)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// getMetricsConfig renders the given template and returns the metrics config.
// Just returns metrics config if no template specified.
func (e *Executor) getMetricsConfig(templatableCfg *config.TemplatableAnalysisMetrics, templateCfg *config.AnalysisTemplateSpec, args map[string]string) (*config.AnalysisMetrics, error) {
	name := templatableCfg.Template.Name
	if name == "" {
		return &templatableCfg.AnalysisMetrics, nil
	}

	var err error
	templateCfg, err = e.render(*templateCfg, args)
	if err != nil {
		return nil, err
	}
	cfg, ok := templateCfg.Metrics[name]
	if !ok {
		return nil, fmt.Errorf("analysis template %s not found despite template specified", name)
	}
	return &cfg, nil
}

// getLogConfig renders the given template and returns the log config.
// Just returns log config if no template specified.
func (e *Executor) getLogConfig(templatableCfg *config.TemplatableAnalysisLog, templateCfg *config.AnalysisTemplateSpec, args map[string]string) (*config.AnalysisLog, error) {
	name := templatableCfg.Template.Name
	if name == "" {
		return &templatableCfg.AnalysisLog, nil
	}

	var err error
	templateCfg, err = e.render(*templateCfg, args)
	if err != nil {
		return nil, err
	}
	cfg, ok := templateCfg.Logs[name]
	if !ok {
		return nil, fmt.Errorf("analysis template %s not found despite template specified", name)
	}
	return &cfg, nil
}

// getHTTPConfig renders the given template and returns the http config.
// Just returns http config if no template specified.
func (e *Executor) getHTTPConfig(templatableCfg *config.TemplatableAnalysisHTTP, templateCfg *config.AnalysisTemplateSpec, args map[string]string) (*config.AnalysisHTTP, error) {
	name := templatableCfg.Template.Name
	if name == "" {
		return &templatableCfg.AnalysisHTTP, nil
	}

	var err error
	templateCfg, err = e.render(*templateCfg, args)
	if err != nil {
		return nil, err
	}
	cfg, ok := templateCfg.HTTPs[name]
	if !ok {
		return nil, fmt.Errorf("analysis template %s not found despite template specified", name)
	}
	return &cfg, nil
}

// render returns a new AnalysisTemplateSpec, where deployment-specific arguments populated.
func (e *Executor) render(templateCfg config.AnalysisTemplateSpec, customArgs map[string]string) (*config.AnalysisTemplateSpec, error) {
	args := templateArgs{
		Args: customArgs,
		App: struct {
			Name string
			Env  string
			// TODO: Populate Env
		}{Name: e.Application.Name, Env: ""},
	}
	if e.config.Kind == config.KindKubernetesApp {
		namespace := "default"
		if n := e.config.KubernetesDeploymentSpec.Input.Namespace; n != "" {
			namespace = n
		}
		args.K8s = struct{ Namespace string }{Namespace: namespace}
	}

	cfg, err := json.Marshal(templateCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}
	t, err := template.New("AnalysisTemplate").Parse(string(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to parse text: %w", err)
	}
	b := new(bytes.Buffer)
	if err := t.Execute(b, args); err != nil {
		return nil, fmt.Errorf("failed to apply template: %w", err)
	}
	newCfg := &config.AnalysisTemplateSpec{}
	err = json.Unmarshal(b.Bytes(), newCfg)
	return newCfg, err
}
