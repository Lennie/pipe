import { DeepPartial } from "@reduxjs/toolkit";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter } from "react-router";
import { createStore, render, screen, waitFor } from "../../test-utils";
import { server } from "../mocks/server";
import { AppState } from "../modules";
import { syncApplication } from "../modules/applications";
import { SyncStrategy } from "../modules/deployments";
import { dummyApplication } from "../__fixtures__/dummy-application";
import { dummyApplicationLiveState } from "../__fixtures__/dummy-application-live-state";
import { dummyEnv } from "../__fixtures__/dummy-environment";
import { dummyPiped } from "../__fixtures__/dummy-piped";
import { ApplicationDetail } from "./application-detail";

beforeAll(() => {
  server.listen();
});

afterEach(() => {
  server.resetHandlers();
});

afterAll(() => {
  server.close();
});

const baseState: DeepPartial<AppState> = {
  applications: {
    ids: [dummyApplication.id],
    entities: {
      [dummyApplication.id]: dummyApplication,
    },
    adding: false,
    disabling: {},
    loading: false,
    syncing: {},
  },
  applicationLiveState: {
    ids: [dummyApplicationLiveState.applicationId],
    entities: {
      [dummyApplicationLiveState.applicationId]: dummyApplicationLiveState,
    },
    hasError: {},
  },
  environments: {
    entities: {
      [dummyEnv.id]: dummyEnv,
    },
    ids: [dummyEnv.id],
  },
  pipeds: {
    entities: {
      [dummyPiped.id]: dummyPiped,
    },
    ids: [dummyPiped.id],
  },
};

describe("ApplicationDetail", () => {
  it("shows application detail and live state", () => {
    const store = createStore(baseState);
    render(
      <MemoryRouter>
        <ApplicationDetail applicationId={dummyApplication.id} />
      </MemoryRouter>,
      {
        store,
      }
    );

    expect(screen.getByText(dummyApplication.name)).toBeInTheDocument();
    expect(screen.getByText("Healthy")).toBeInTheDocument();
    expect(screen.getByText("Synced")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sync$/i })).toBeInTheDocument();
  });

  describe("sync", () => {
    it("dispatch sync action if click sync button", async () => {
      const store = createStore(baseState);
      render(
        <MemoryRouter>
          <ApplicationDetail applicationId={dummyApplication.id} />
        </MemoryRouter>,
        {
          store,
        }
      );

      userEvent.click(screen.getByRole("button", { name: /sync$/i }));

      await waitFor(() =>
        expect(store.getActions()).toMatchObject([
          {
            type: syncApplication.pending.type,
            meta: {
              arg: {
                applicationId: dummyApplication.id,
                syncStrategy: SyncStrategy.AUTO,
              },
            },
          },
        ])
      );
    });

    it("dispatch sync action with selected sync strategy if changed strategy and click the sync button", async () => {
      const store = createStore(baseState);
      render(
        <MemoryRouter>
          <ApplicationDetail applicationId={dummyApplication.id} />
        </MemoryRouter>,
        {
          store,
        }
      );

      userEvent.click(
        screen.getByRole("button", { name: /select sync strategy/i })
      );
      userEvent.click(screen.getByRole("menuitem", { name: /pipeline sync/i }));
      userEvent.click(screen.getByRole("button", { name: /pipeline sync/i }));

      await waitFor(() =>
        expect(store.getActions()).toMatchObject([
          {
            type: syncApplication.pending.type,
            meta: {
              arg: {
                applicationId: dummyApplication.id,
                syncStrategy: SyncStrategy.PIPELINE,
              },
            },
          },
        ])
      );
    });
  });
});
