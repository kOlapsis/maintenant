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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestListAlerts_CutsTheURLsOfStoredAlerts(t *testing.T) {
	f := newAlertFixture(t)
	_, err := f.alerts.InsertAlert(context.Background(), &alert.Alert{
		Source: alert.SourceEndpoint, AlertType: "consecutive_failure", Severity: alert.SeverityCritical,
		Status: alert.StatusActive, EntityType: "endpoint", EntityID: "ep-1", EntityName: "api", FiredAt: time.Now(),
		Message: "Endpoint https://ops:hunter2@example.com/hooks/S3CR3T?token=t0k3n failed 3 consecutive checks",
		Details: `{"target":"https://ops:hunter2@example.com/hooks/S3CR3T?token=t0k3n","last_error":"Get \"https://ops:***@example.com/hooks/S3CR3T?token=t0k3n\": EOF"}`,
	})
	require.NoError(t, err)

	for _, activeOnly := range []bool{true, false} {
		result, _, err := listAlertsHandler(f.svc)(context.Background(), nil, listAlertsInput{ActiveOnly: &activeOnly})
		require.NoError(t, err)
		text := textFromContent(t, result.Content)
		for _, secret := range []string{"hunter2", "ops:", "S3CR3T", "t0k3n"} {
			assert.NotContains(t, text, secret)
		}
		var rows []struct {
			Message string `json:"message"`
			Details string `json:"details"`
		}
		require.NoError(t, json.Unmarshal([]byte(text), &rows))
		require.Len(t, rows, 1)
		assert.Equal(t, "Endpoint https://example.com failed 3 consecutive checks", rows[0].Message)
		assert.JSONEq(t, `{"target":"https://example.com","last_error":"Get \"https://example.com\": EOF"}`, rows[0].Details)
	}
}

func TestGetEndpointHistory_CutsTheURLsOfStoredErrors(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eps := store.NewEndpointStore(storetest.Open(t, logger))
	svc := &Services{
		Endpoints: endpoint.NewService(endpoint.Deps{Store: eps, Engine: endpoint.NewCheckEngine(nil, logger), Logger: logger}),
		Logger:    logger,
	}
	ep, err := svc.Endpoints.CreateStandalone(ctx, "api", "https://example.com/health", endpoint.TypeHTTP, endpoint.DefaultConfig())
	require.NoError(t, err)
	_, err = eps.InsertCheckResult(ctx, &endpoint.CheckResult{
		EndpointID: ep.ID, Timestamp: time.Now(),
		ErrorMessage: `request failed: Get "https://ops:***@example.com/hooks/S3CR3T?token=t0k3n": EOF`,
	})
	require.NoError(t, err)

	result, _, err := getEndpointHistoryHandler(svc)(ctx, nil, getEndpointHistoryInput{EndpointID: ep.ID})
	require.NoError(t, err)
	var rows []struct {
		ErrorMessage string `json:"error_message"`
	}
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, `request failed: Get "https://example.com": EOF`, rows[0].ErrorMessage)
}
