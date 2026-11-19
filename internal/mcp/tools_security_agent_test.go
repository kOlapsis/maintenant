// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

const homonymWorkload = "shop/Deployment/api"

// Two clusters run the same workload; the container_id of list_containers reads the CVEs and the risk of its own cluster.
func TestSecurityTools_ContainerIDReadsItsOwnAgent(t *testing.T) {
	withEdition(t, extension.Pro)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	containerStore := store.NewContainerStore(db)
	updateStore := store.NewUpdateStore(db)
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)

	now := time.Now()
	ids := map[string]string{}
	for agentID, risk := range map[string]int{"agent-staging": 20, "agent-prod": 90} {
		require.NoError(t, agentStore.Insert(ctx, &agent.Agent{AgentID: agentID, Hostname: agentID, Status: "active",
			DetectedRuntime: "kubernetes", CreatedAt: now}))
		id, err := containerStore.InsertContainer(ctx, &container.Container{
			ExternalID: homonymWorkload, AgentID: agentID, Name: "api", Image: "shop/api:1.0.0",
			State: container.StateRunning, AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
			RuntimeType: "kubernetes", FirstSeenAt: now, LastStateChangeAt: now,
		})
		require.NoError(t, err)
		ids[agentID] = id
		_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
			ScanID: scanID, ContainerID: homonymWorkload, AgentID: agentID, ContainerName: "api",
			Image: "shop/api:1.0.0", CurrentTag: "1.0.0", Registry: "ghcr.io", LatestTag: "1.1.0",
			UpdateType: update.UpdateTypeMinor, RiskScore: risk, Status: update.StatusAvailable, DetectedAt: now,
		})
		require.NoError(t, err)
		_, err = db.ReadDB().ExecContext(ctx, db.Dialect().Rebind(
			`INSERT INTO container_cves (id, agent_id, container_id, cve_id, severity, first_detected_at)
			VALUES (?, ?, ?, ?, 'high', ?)`), uid.New(), agentID, homonymWorkload, "CVE-"+agentID, now.Unix())
		require.NoError(t, err)
	}

	svc := &Services{
		Containers:  container.NewService(container.Deps{Store: containerStore, Logger: logger}),
		UpdateStore: updateStore,
		Updates: update.NewService(update.Deps{
			Store:      updateStore,
			Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: noContainers{},
			Logger:     logger,
		}),
		Agents: agentStore,
		Logger: logger,
	}

	for agentID, risk := range map[string]int{"agent-staging": 20, "agent-prod": 90} {
		result, _, err := listCVEHandler(svc)(ctx, nil, listCVEInput{ContainerID: ids[agentID]})
		require.NoError(t, err)
		require.False(t, result.IsError, textFromContent(t, result.Content))
		var cves struct {
			CVEs []struct {
				CVEID string `json:"cve_id"`
			} `json:"cves"`
		}
		require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &cves))
		require.Len(t, cves.CVEs, 1, "CVEs of %s", agentID)
		assert.Equal(t, "CVE-"+agentID, cves.CVEs[0].CVEID)

		result, _, err = listRiskScoresHandler(svc)(ctx, nil, listRiskScoresInput{ContainerID: ids[agentID]})
		require.NoError(t, err)
		require.False(t, result.IsError, textFromContent(t, result.Content))
		var row struct {
			AgentID   string `json:"agent_id"`
			RiskScore int    `json:"risk_score"`
		}
		require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &row))
		assert.Equal(t, agentID, row.AgentID)
		assert.Equal(t, risk, row.RiskScore)
	}

	cveResult, _, err := listCVEHandler(svc)(ctx, nil, listCVEInput{ContainerID: homonymWorkload})
	require.NoError(t, err)
	assert.True(t, cveResult.IsError, "an external id two agents run cannot pick one")
	assert.Contains(t, textFromContent(t, cveResult.Content), "list_containers")
	riskResult, _, err := listRiskScoresHandler(svc)(ctx, nil, listRiskScoresInput{ContainerID: homonymWorkload})
	require.NoError(t, err)
	assert.True(t, riskResult.IsError, "an external id two agents run cannot pick one")
}
