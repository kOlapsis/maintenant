// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestHandleCheckNow_AnswersItselfWhenTheCheckOutlastsTheRequest(t *testing.T) {
	prev := manualCheckWait
	manualCheckWait = 20 * time.Millisecond
	t.Cleanup(func() { manualCheckWait = prev })

	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(target.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	endpoints := store.NewEndpointStore(storetest.Open(t, logger))
	svc := endpoint.NewService(endpoint.Deps{Store: endpoints, Engine: endpoint.NewCheckEngine(nil, logger), Logger: logger})
	t.Cleanup(svc.Stop)
	id, err := endpoints.UpsertEndpoint(context.Background(), &endpoint.Endpoint{
		ExternalID: "ctr-slow", ContainerName: "slow", LabelKey: "k1",
		EndpointType: endpoint.TypeHTTP, Target: target.URL,
		Status: endpoint.StatusUnknown, AlertState: endpoint.AlertNormal,
		Config: endpoint.DefaultConfig(), Active: true, AgentID: uid.LocalAgent,
	})
	require.NoError(t, err)
	h := NewEndpointHandler(svc, nil)

	rec := sendEndpoint(h.HandleCheckNow, http.MethodPost, "/api/v1/endpoints/"+id+"/check", id, "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var body struct {
		Pending  bool `json:"pending"`
		Endpoint struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"endpoint"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Pending)
	assert.Equal(t, id, body.Endpoint.ID)
	assert.Equal(t, "unknown", body.Endpoint.Status)

	close(release)
	require.Eventually(t, func() bool {
		ep, err := svc.GetEndpoint(context.Background(), id)
		return err == nil && ep != nil && ep.Status == endpoint.StatusUp
	}, 5*time.Second, 10*time.Millisecond, "the check that outlasted the request must still be recorded")
}
