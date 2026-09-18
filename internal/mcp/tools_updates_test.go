// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/update"
)

type noContainers struct{}

func (noContainers) ListContainerInfos(context.Context) ([]update.ContainerInfo, error) {
	return nil, nil
}

// TestGetUpdates_CarriesHostsAndTable pins the three-key shape of get_updates:
// the image updates, every monitored host's operating system, and the table the
// support dates come from.
func TestGetUpdates_CarriesHostsAndTable(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "web-03", Hostname: "web-03", Label: "web-03", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))
	today := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	_, err := agentStore.UpdateAgentOS(ctx, "web-03", agent.OSIdentity{
		ID: "debian", VersionID: "11", PrettyName: "Debian GNU/Linux 11 (bullseye)", Source: "host_file",
	}, today)
	require.NoError(t, err)

	eolSvc, err := eol.New(eol.Deps{
		Store:  agentStore,
		Emit:   func(alert.Event) {},
		Now:    func() time.Time { return today },
		Logger: logger,
	})
	require.NoError(t, err)

	updateStore := store.NewUpdateStore(db)
	svc := &Services{
		Updates: update.NewService(update.Deps{
			Store:      updateStore,
			Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: noContainers{},
			Logger:     logger,
		}),
		EOL:    eolSvc,
		Logger: logger,
	}

	result, _, err := getUpdatesHandler(svc)(ctx, nil, getUpdatesInput{})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*gomcp.TextContent)
	require.True(t, ok)

	var payload struct {
		Updates []any `json:"updates"`
		Hosts   []struct {
			AgentID string `json:"agent_id"`
			OS      struct {
				Support struct {
					State string `json:"state"`
				} `json:"support"`
			} `json:"os"`
		} `json:"hosts"`
		EolTable struct {
			Source string `json:"source"`
		} `json:"eol_table"`
	}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))

	require.Len(t, payload.Hosts, 2, "the local sentinel is listed beside the enrolled host")
	assert.Equal(t, "web-03", payload.Hosts[0].AgentID)
	assert.Equal(t, "ended", payload.Hosts[0].OS.Support.State)
	assert.Equal(t, eol.SourceEmbedded, payload.EolTable.Source)
}
