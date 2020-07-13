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

package kubernetes

import (
	"context"
	"fmt"

	"github.com/pipe-cd/pipe/pkg/model"
)

// NOTE: Just copying from ensurePrimaryUpdate. I will update the implementation in another PR.
func (e *Executor) ensureSync(ctx context.Context) model.StageStatus {
	manifests, err := e.loadManifests(ctx)
	if err != nil {
		e.LogPersister.AppendError(fmt.Sprintf("Failed while loading manifests (%v)", err))
		return model.StageStatus_STAGE_FAILURE
	}

	if len(manifests) == 0 {
		e.LogPersister.AppendError("There are no kubernetes manifests to handle")
		return model.StageStatus_STAGE_FAILURE
	}

	primaryManifests, err := e.generatePrimaryManifests(e.config.Input.Namespace, e.Deployment.Trigger.Commit.Hash, manifests)
	if err != nil {
		e.LogPersister.AppendError(fmt.Sprintf("Unable to generate manifests for PRIMARY variant (%v)", err))
		return model.StageStatus_STAGE_FAILURE
	}

	e.LogPersister.AppendInfo(fmt.Sprintf("Applying %d primary resources", len(primaryManifests)))
	for _, m := range primaryManifests {
		if err = e.provider.ApplyManifest(ctx, m); err != nil {
			e.LogPersister.AppendError(fmt.Sprintf("Failed to apply manifest: %s (%v)", m.Key.ReadableString(), err))
			return model.StageStatus_STAGE_FAILURE
		}
		e.LogPersister.AppendSuccess(fmt.Sprintf("- applied manifest: %s", m.Key.ReadableString()))
	}

	e.LogPersister.AppendSuccess(fmt.Sprintf("Successfully applied %d primary resources", len(primaryManifests)))
	return model.StageStatus_STAGE_SUCCESS
}