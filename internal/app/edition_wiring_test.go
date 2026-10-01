// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
)

func TestEdition_ReportsTheStatusURL(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) { c.StatusURL = "https://status.example.com" })

	rec := serve(a, http.MethodGet, "/api/v1/edition", "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		StatusURL string `json:"status_url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "https://status.example.com", body.StatusURL)
}

// The license manager settles the edition after New and may change it later; the caps follow.
func TestQuotas_FollowTheRunningEdition(t *testing.T) {
	a, _ := newTestApp(t, nil)
	ctx := context.Background()
	prev := extension.CurrentEdition
	t.Cleanup(func() { extension.CurrentEdition = prev })
	setEdition := func(e extension.Edition) { extension.CurrentEdition = func() extension.Edition { return e } }
	setEdition(extension.Community)

	create := map[extension.Resource]func(i int) error{
		extension.ResourceEndpoints: func(i int) error {
			_, err := a.endpointSvc.CreateStandalone(ctx, fmt.Sprintf("ep%d", i), fmt.Sprintf("https://ep%d.example.com", i),
				endpoint.TypeHTTP, endpoint.DefaultConfig())
			return err
		},
		extension.ResourceHeartbeats: func(i int) error {
			_, err := a.heartbeatSvc.CreateHeartbeat(ctx, heartbeat.CreateHeartbeatInput{
				Name: fmt.Sprintf("job%d", i), IntervalSeconds: 3600, GraceSeconds: 60,
			}, fmt.Sprintf("token-%d", i))
			return err
		},
		extension.ResourceCertificates: func(i int) error {
			_, _, err := a.certSvc.CreateStandalone(ctx, certificate.CreateCertificateInput{
				Hostname: fmt.Sprintf("cert%d.invalid", i), Port: 443,
			})
			return err
		},
	}
	limitErr := map[extension.Resource]error{
		extension.ResourceEndpoints:    endpoint.ErrLimitReached,
		extension.ResourceHeartbeats:   heartbeat.ErrLimitReached,
		extension.ResourceCertificates: certificate.ErrLimitReached,
	}

	for r, fn := range create {
		setEdition(extension.Community)
		limit := extension.Limit(r)
		require.Positive(t, limit, r)
		for i := range limit {
			require.NoError(t, fn(i), "%s %d", r, i)
		}
		require.ErrorIs(t, fn(limit), limitErr[r], "%s over the Community cap", r)

		setEdition(extension.Personal)
		assert.NoError(t, fn(limit), "%s under Personal", r)

		setEdition(extension.Community)
		assert.ErrorIs(t, fn(limit+1), limitErr[r], "%s after a downgrade", r)
	}
}
