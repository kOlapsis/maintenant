// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestListHeartbeats_NeverReturnsAPingToken(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &Services{
		Heartbeats: heartbeat.NewService(heartbeat.Deps{Store: store.NewHeartbeatStore(storetest.Open(t, logger)), Logger: logger}),
		Logger:     logger,
	}
	hb, err := svc.Heartbeats.CreateHeartbeat(ctx, heartbeat.CreateHeartbeatInput{Name: "nightly", IntervalSeconds: 3600, GraceSeconds: 60}, "tok-secret-0123456789abcdef")
	require.NoError(t, err)

	result, _, err := listHeartbeatsHandler(svc)(ctx, nil, listHeartbeatsInput{})
	require.NoError(t, err)
	text := textFromContent(t, result.Content)

	assert.NotContains(t, text, "tok-secret-0123456789abcdef")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &rows))
	require.Len(t, rows, 1)
	assert.NotContains(t, rows[0], "ping_token")
	assert.Equal(t, hb.ID, rows[0]["id"])
	assert.Equal(t, "nightly", rows[0]["name"])
}

func TestListEndpoints_MasksProbeHeaderValues(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &Services{
		Endpoints: endpoint.NewService(endpoint.Deps{
			Store:  store.NewEndpointStore(storetest.Open(t, logger)),
			Engine: endpoint.NewCheckEngine(nil, logger),
			Logger: logger,
		}),
		Logger: logger,
	}
	cfg := endpoint.DefaultConfig()
	cfg.Headers = map[string]string{"Authorization": "Bearer s3cr3t-value", "X-Trace": "abc"}
	_, err := svc.Endpoints.CreateStandalone(ctx, "api", "https://example.com/health", endpoint.TypeHTTP, cfg)
	require.NoError(t, err)

	result, _, err := listEndpointsHandler(svc)(ctx, nil, listEndpointsInput{})
	require.NoError(t, err)
	text := textFromContent(t, result.Content)

	assert.NotContains(t, text, "s3cr3t-value")
	assert.NotContains(t, text, `"abc"`)
	var rows []struct {
		Name   string `json:"name"`
		Config struct {
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		} `json:"config"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "api", rows[0].Name)
	assert.Equal(t, "GET", rows[0].Config.Method)
	assert.Equal(t, map[string]string{"Authorization": "***", "X-Trace": "***"}, rows[0].Config.Headers)
}

func TestEndpointViews_CutTheTargetAfterTheHost(t *testing.T) {
	views := endpointViews([]*endpoint.Endpoint{
		{
			Target:    "https://ops:hunter2@example.com:8443/hooks/S3CR3T?token=t0k3n",
			LastError: `request failed: Get "https://ops:***@example.com:8443/hooks/S3CR3T?token=t0k3n": stopped after 3 redirects to https://sso.example.com/cb?code=C0DE`,
		},
		{Target: "db.internal:5432", LastError: "tcp dial failed: dial tcp 10.0.0.7:5432: connect: connection refused"},
	})
	text, err := json.Marshal(views)
	require.NoError(t, err)

	for _, secret := range []string{"hunter2", "ops:", "S3CR3T", "t0k3n", "C0DE"} {
		assert.NotContains(t, string(text), secret)
	}
	var rows []struct {
		Target    string `json:"target"`
		LastError string `json:"last_error"`
	}
	require.NoError(t, json.Unmarshal(text, &rows))
	assert.Equal(t, "https://example.com:8443", rows[0].Target)
	assert.Equal(t, `request failed: Get "https://example.com:8443": stopped after 3 redirects to https://sso.example.com`, rows[0].LastError)
	assert.Equal(t, "db.internal:5432", rows[1].Target)
	assert.Equal(t, "tcp dial failed: dial tcp 10.0.0.7:5432: connect: connection refused", rows[1].LastError)
}
