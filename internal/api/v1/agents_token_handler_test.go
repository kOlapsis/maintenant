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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

// newTokenTestHandler wires an AgentHandler over a temp database, at an edition
// that opens multi-host (Community refuses enrollment outright).
func newTokenTestHandler(t *testing.T) (*AgentHandler, *store.AgentStore) {
	t.Helper()

	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	store := store.NewAgentStore(db)
	return NewAgentHandler(store, nil, nil, logger, "grpcs://example.test:8443", "127.0.0.1:8443", time.Minute, nil), store
}

func createToken(t *testing.T, h *AgentHandler) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enrollment-tokens",
		strings.NewReader(`{"ttl_hours":1}`))
	h.HandleCreateEnrollmentToken(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// The creation response is the one and only place the cleartext appears. It
// must still carry a usable token and the install commands built from it.
func TestCreateEnrollmentToken_ReturnsCleartextOnce(t *testing.T) {
	h, store := newTokenTestHandler(t)
	out := createToken(t, h)

	cleartext, _ := out["token"].(string)
	require.True(t, strings.HasPrefix(cleartext, "mnt_enr_"), "got %q", cleartext)
	assert.Equal(t, agent.TokenPrefix(cleartext)+"...***", out["token_masked"])
	assert.Equal(t, agent.TokenIDFromHash(agent.HashToken(cleartext)), out["token_id"])

	// The install snippets have to embed the real token or the operator cannot
	// copy-paste them.
	templates, ok := out["install_templates"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, templates)
	for name, tmpl := range templates {
		assert.Contains(t, tmpl, cleartext, "install template %q must carry the token", name)
	}

	// And the stored row holds the hash, never the token.
	stored, err := store.GetByToken(context.Background(), cleartext)
	require.NoError(t, err)
	assert.Equal(t, agent.HashToken(cleartext), stored.TokenHash)
	assert.NotContains(t, stored.TokenHash, "mnt_enr_")
}

// Every read path after creation shows the masked form and nothing more. This
// is what keeps a leaked API response from being replayable.
func TestEnrollmentTokenReadPaths_NeverReturnCleartext(t *testing.T) {
	h, _ := newTokenTestHandler(t)
	out := createToken(t, h)
	cleartext := out["token"].(string)
	tokenID := out["token_id"].(string)

	t.Run("list", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.HandleListEnrollmentTokens(rec,
			httptest.NewRequest(http.MethodGet, "/api/v1/agents/enrollment-tokens", nil))
		require.Equal(t, http.StatusOK, rec.Code)

		assert.NotContains(t, rec.Body.String(), cleartext)
		assert.Contains(t, rec.Body.String(), agent.TokenPrefix(cleartext)+"...***")
	})

	t.Run("get by id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/enrollment-tokens/"+tokenID, nil)
		req.SetPathValue("token_id", tokenID)
		h.HandleGetEnrollmentToken(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		assert.NotContains(t, rec.Body.String(), cleartext)
		assert.Contains(t, rec.Body.String(), agent.TokenPrefix(cleartext)+"...***")
	})
}

// Two tokens minted back to back must not collide on their id or their prefix
// display, or the list becomes ambiguous once the cleartext is gone.
func TestCreateEnrollmentToken_TokensAreDistinct(t *testing.T) {
	h, _ := newTokenTestHandler(t)
	first := createToken(t, h)
	second := createToken(t, h)

	assert.NotEqual(t, first["token"], second["token"])
	assert.NotEqual(t, first["token_id"], second["token_id"])
	assert.NotEqual(t, first["token_masked"], second["token_masked"],
		"the stored prefix must still tell two tokens apart in the list")
}

func TestCreateEnrollmentToken_AHugeTTLIsCappedNotExpired(t *testing.T) {
	h, _ := newTokenTestHandler(t)
	rec := httptest.NewRecorder()
	h.HandleCreateEnrollmentToken(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agents/enrollment-tokens",
		strings.NewReader(`{"ttl_hours":3000000}`)))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var out struct {
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.WithinDuration(t, time.Now().Add(168*time.Hour), out.ExpiresAt, time.Minute)
}

func TestRevokeAgent_ASecondRevocationChangesNothing(t *testing.T) {
	h, agents := newTokenTestHandler(t)
	h.broker = NewSSEBroker(h.logger)
	events := make(chan SSEEvent, 4)
	h.broker.AddObserver(events)
	require.NoError(t, agents.Insert(context.Background(), &agent.Agent{
		AgentID: "web-01", PublicKey: []byte("pubkeyplaceholder12345678901234"), Hostname: "web-01",
		OSArch: "linux/amd64", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now(),
	}))
	revoke := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+id+"/revoke", nil)
		req.SetPathValue("id", id)
		h.HandleRevokeAgent(rec, req)
		return rec
	}

	first := revoke("web-01")
	require.Equal(t, http.StatusOK, first.Code)
	assert.Contains(t, first.Body.String(), `"runtime_unreachable":false`)
	require.Len(t, events, 1)

	second := revoke("web-01")
	assert.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"status":"revoked"`)
	assert.Contains(t, second.Body.String(), `"runtime_unreachable":false`)
	assert.Len(t, events, 1, "nothing is announced twice")

	assert.Equal(t, http.StatusNotFound, revoke(uid.LocalAgent).Code)
	assert.Equal(t, http.StatusNotFound, revoke("unknown").Code)
}
