// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 043-public-surfaces-hardening, US2 (C2).

func heartbeatRouter(t *testing.T) (http.Handler, *heartbeat.Service) {
	t.Helper()
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := heartbeat.NewService(heartbeat.Deps{Store: store.NewHeartbeatStore(storetest.Open(t, logger)), Logger: logger})
	return NewRouter(HandlerDeps{Logger: logger, Heartbeats: svc}).Handler(), svc
}

func createHeartbeat(t *testing.T, admin http.Handler, name string) heartbeat.Heartbeat {
	t.Helper()
	rec := serve(t, admin, http.MethodPost, "/api/v1/heartbeats", `{"name":"`+name+`","interval_seconds":300,"grace_seconds":60}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var hb heartbeat.Heartbeat
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hb))
	return hb
}

func heartbeatErrorCode(t *testing.T, rec interface{ Bytes() []byte }) string {
	t.Helper()
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Bytes(), &body))
	return body.Error.Code
}

func getHeartbeat(t *testing.T, admin http.Handler, id string) heartbeat.Heartbeat {
	t.Helper()
	rec := serve(t, admin, http.MethodGet, "/api/v1/heartbeats/"+id, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var detail struct {
		Heartbeat heartbeat.Heartbeat `json:"heartbeat"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	return detail.Heartbeat
}

func pingCount(t *testing.T, svc *heartbeat.Service, id string) int {
	t.Helper()
	pings, _, err := svc.ListPings(context.Background(), id, heartbeat.ListPingsOpts{Limit: 100})
	require.NoError(t, err)
	return len(pings)
}

// US2 scenario 1, FR-010, FR-013: a heartbeat carries a ping token that is not its id, on every admin surface.
func TestHeartbeats_PingTokenIsASecretDistinctFromTheID(t *testing.T) {
	admin, _ := heartbeatRouter(t)

	var created []heartbeat.Heartbeat
	for _, name := range []string{"nightly backup", "hourly sync", "weekly report"} {
		hb := createHeartbeat(t, admin, name)
		assert.NotEmpty(t, hb.ID)
		assert.NotEmpty(t, hb.PingToken, "%s: the created heartbeat carries its ping token", name)
		assert.NotEqual(t, hb.ID, hb.PingToken, "%s: the token is not the id", name)
		assert.Equal(t, hb.PingToken, getHeartbeat(t, admin, hb.ID).PingToken, "%s: the detail carries the same token", name)
		created = append(created, hb)
	}
	seen := map[string]bool{}
	for _, hb := range created {
		assert.False(t, seen[hb.PingToken], "two heartbeats never share a token")
		seen[hb.PingToken] = true
	}

	rec := serve(t, admin, http.MethodGet, "/api/v1/heartbeats", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list struct {
		Heartbeats []heartbeat.Heartbeat `json:"heartbeats"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Heartbeats, 3)
	for _, hb := range list.Heartbeats {
		assert.True(t, seen[hb.PingToken], "the list carries the ping token of %s", hb.Name)
		assert.NotEqual(t, hb.ID, hb.PingToken)
	}
}

// US2 scenario 2, FR-010: /ping resolves by token, in its three forms, and never by id.
func TestPing_ResolvesByTokenAndNeverByID(t *testing.T) {
	admin, svc := heartbeatRouter(t)
	hb := createHeartbeat(t, admin, "nightly backup")

	for _, suffix := range []string{"", "/start", "/0"} {
		rec := serve(t, admin, http.MethodGet, "/ping/"+hb.PingToken+suffix, "")
		assert.Equal(t, http.StatusOK, rec.Code, "token%s: %s", suffix, rec.Body.String())

		rec = serve(t, admin, http.MethodPost, "/ping/"+hb.ID+suffix, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, "id%s must be unknown: %s", suffix, rec.Body.String())
		assert.Equal(t, "HEARTBEAT_NOT_FOUND", heartbeatErrorCode(t, rec.Body))
	}

	assert.Equal(t, 3, pingCount(t, svc, hb.ID), "one ping recorded per accepted call, none for the id")
	assert.Equal(t, heartbeat.StatusUp, getHeartbeat(t, admin, hb.ID).Status)

	rec := serve(t, admin, http.MethodGet, "/ping/"+uid.New(), "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "HEARTBEAT_NOT_FOUND", heartbeatErrorCode(t, rec.Body))
}

// US2 scenarios 3 and 5, FR-012, SC-003: rotation replaces the token at once and keeps id, state and history.
func TestHeartbeats_RotateTokenInvalidatesTheOldOneAndKeepsTheHistory(t *testing.T) {
	admin, svc := heartbeatRouter(t)
	a := createHeartbeat(t, admin, "nightly backup")
	b := createHeartbeat(t, admin, "hourly sync")

	for i := 0; i < 2; i++ {
		rec := serve(t, admin, http.MethodGet, "/ping/"+a.PingToken, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	before := getHeartbeat(t, admin, a.ID)

	rec := serve(t, admin, http.MethodPost, "/api/v1/heartbeats/"+a.ID+"/rotate-token", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var rotated heartbeat.Heartbeat
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rotated))
	assert.Equal(t, a.ID, rotated.ID, "the id does not change")
	assert.NotEmpty(t, rotated.PingToken)
	assert.NotEqual(t, a.PingToken, rotated.PingToken, "the token changes")
	assert.NotEqual(t, a.ID, rotated.PingToken)
	assert.NotEqual(t, b.PingToken, rotated.PingToken)
	assert.Equal(t, before.Status, rotated.Status, "the state is kept")
	assert.Equal(t, before.Name, rotated.Name)

	rec = serve(t, admin, http.MethodGet, "/ping/"+a.PingToken, "")
	assert.Equal(t, http.StatusNotFound, rec.Code, "the old token is refused from the next request on: %s", rec.Body.String())
	assert.Equal(t, "HEARTBEAT_NOT_FOUND", heartbeatErrorCode(t, rec.Body))

	rec = serve(t, admin, http.MethodGet, "/ping/"+rotated.PingToken, "")
	assert.Equal(t, http.StatusOK, rec.Code, "the new token is accepted: %s", rec.Body.String())

	rec = serve(t, admin, http.MethodGet, "/ping/"+b.PingToken, "")
	assert.Equal(t, http.StatusOK, rec.Code, "the other heartbeat is not affected: %s", rec.Body.String())
	assert.Equal(t, b.PingToken, getHeartbeat(t, admin, b.ID).PingToken)

	after := getHeartbeat(t, admin, a.ID)
	assert.Equal(t, rotated.PingToken, after.PingToken)
	assert.Equal(t, heartbeat.StatusUp, after.Status)
	assert.Equal(t, 3, pingCount(t, svc, a.ID), "two pings before the rotation and one after: the history is intact")

	rec = serve(t, admin, http.MethodPost, "/api/v1/heartbeats/"+a.ID+"/rotate-token", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var again heartbeat.Heartbeat
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	assert.NotEqual(t, rotated.PingToken, again.PingToken, "every rotation yields a fresh token")
	assert.Equal(t, a.ID, again.ID)

	rec = serve(t, admin, http.MethodPost, "/api/v1/heartbeats/00000000-0000-0000-0000-00000000dead/rotate-token", "")
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "HEARTBEAT_NOT_FOUND", heartbeatErrorCode(t, rec.Body))
}

// Edge case: rotation is a write, so demo mode refuses it like every other write.
// Passes on the skeleton already: the guard is existing behaviour and the test pins the new route behind it.
func TestHeartbeats_RotateTokenIsRefusedInDemoMode(t *testing.T) {
	admin, svc := heartbeatRouter(t)
	hb := createHeartbeat(t, admin, "nightly backup")
	guarded := DemoModeGuard(true, "demo-driver-token")(admin)

	rec := serve(t, guarded, http.MethodPost, "/api/v1/heartbeats/"+hb.ID+"/rotate-token", "")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, "DEMO_MODE", heartbeatErrorCode(t, rec.Body))

	got, err := svc.GetHeartbeat(context.Background(), hb.ID)
	require.NoError(t, err)
	assert.Equal(t, hb.PingToken, got.PingToken, "a refused rotation leaves the token in place")
}
