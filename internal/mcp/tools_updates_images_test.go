// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

type listedContainers []update.ContainerInfo

func (l listedContainers) ListContainerInfos(context.Context) ([]update.ContainerInfo, error) {
	return l, nil
}

type endedPostgres struct{}

func (endedPostgres) Current() eol.Table {
	ended, _ := eol.ParseDate("2020-01-01")
	return eol.Table{Source: "test", Products: map[string][]eol.Cycle{"postgresql": {{Name: "9", SecurityUntil: ended}}}}
}

func TestGetUpdates_CarriesTheSupportOfRunningImages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "agent-nas", Hostname: "nas-02", Label: "NAS cave", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))

	updateStore := store.NewUpdateStore(db)
	svc := &Services{
		Updates: update.NewService(update.Deps{
			Store:   updateStore,
			Scanner: update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: listedContainers{
				{ExternalID: "ctr-local", Name: "db", Image: "postgres:9.1.2"},
				{ExternalID: "ctr-nas", AgentID: "agent-nas", Name: "db", Image: "postgres:9.6"},
				{ExternalID: "ctr-other", Name: "cache", Image: "redis:7"},
			},
			Logger: logger,
			EOL:    endedPostgres{},
		}),
		Agents: agentStore,
		Logger: logger,
	}

	result, _, err := getUpdatesHandler(svc)(ctx, nil, getUpdatesInput{})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*gomcp.TextContent)
	require.True(t, ok)

	var payload struct {
		Images []map[string]any `json:"images"`
	}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))

	require.Len(t, payload.Images, 2)
	assert.Equal(t, "ctr-local", payload.Images[0]["container_id"])
	assert.Equal(t, uid.LocalAgent, payload.Images[0]["agent_id"])
	assert.Equal(t, "postgres:9.1.2", payload.Images[0]["image"])
	assert.Equal(t, "ended", payload.Images[0]["support"].(map[string]any)["state"])
	assert.Equal(t, "ctr-nas", payload.Images[1]["container_id"])
	assert.Equal(t, "agent-nas", payload.Images[1]["agent_id"])
	assert.Equal(t, "NAS cave", payload.Images[1]["agent_label"])
}

func TestGetUpdates_ImagesIsAnEmptyArrayWithoutTrackedImages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	updateStore := store.NewUpdateStore(db)
	svc := &Services{
		Updates: update.NewService(update.Deps{
			Store:      updateStore,
			Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: noContainers{},
			Logger:     logger,
		}),
		Logger: logger,
	}

	result, _, err := getUpdatesHandler(svc)(ctx, nil, getUpdatesInput{})
	require.NoError(t, err)
	text, ok := result.Content[0].(*gomcp.TextContent)
	require.True(t, ok)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	assert.JSONEq(t, `[]`, string(payload["images"]))
}
