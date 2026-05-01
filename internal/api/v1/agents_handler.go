// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// AgentHandler handles REST endpoints for agents and enrollment tokens.
type AgentHandler struct {
	store       *sqlite.AgentStore
	broker      *SSEBroker
	logger      *slog.Logger
	grpcPublicURL string
	grpcListen  string

	// connectedIDs is populated by the gRPC server (US2) with currently-connected agent IDs.
	// Nil or empty means no live sessions data available — all agents show disconnected.
	connectedIDs func() map[string]struct{}
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(
	store *sqlite.AgentStore,
	broker *SSEBroker,
	logger *slog.Logger,
	grpcPublicURL string,
	grpcListen string,
) *AgentHandler {
	return &AgentHandler{
		store:         store,
		broker:        broker,
		logger:        logger,
		grpcPublicURL: grpcPublicURL,
		grpcListen:    grpcListen,
		connectedIDs:  func() map[string]struct{} { return nil },
	}
}

// SetConnectedIDsFn lets the gRPC server provide live connection state (added in US2).
func (h *AgentHandler) SetConnectedIDsFn(fn func() map[string]struct{}) {
	h.connectedIDs = fn
}

// ─── Enrollment token endpoints ──────────────────────────────────────────────

// HandleCreateEnrollmentToken handles POST /api/v1/agents/enrollment-tokens
func (h *AgentHandler) HandleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TTLHours int `json:"ttl_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	ttl := 24 * time.Hour
	if body.TTLHours > 0 {
		ttl = time.Duration(body.TTLHours) * time.Hour
	}
	if ttl > 168*time.Hour { // max 7 days
		ttl = 168 * time.Hour
	}

	// Generate 32 random bytes, encode as lowercase base32 (no padding), prefix with "mnt_enr_".
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate token")
		return
	}
	tokenStr := "mnt_enr_" + strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="))

	// token_id = first 16 hex chars of SHA-256(token).
	sum := sha256.Sum256([]byte(tokenStr))
	tokenID := hex.EncodeToString(sum[:])[:16]

	now := time.Now().UTC()
	tok := &agent.EnrollmentToken{
		TokenID:   tokenID,
		Token:     tokenStr,
		CreatedBy: "admin",
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := h.store.InsertToken(r.Context(), tok); err != nil {
		h.logger.Error("insert enrollment token", "err", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create token")
		return
	}

	// Resolve the gRPC public URL and build the install command.
	publicURL, warnings := agentserver.ResolvePublicURL(r, agentserver.PublicURLConfig{
		Explicit:   h.grpcPublicURL,
		ListenAddr: h.grpcListen,
	})
	installCmd := "maintenant --mode=agent --server=" + publicURL + " --enrollment-token=" + tokenStr

	WriteJSON(w, http.StatusCreated, map[string]any{
		"token_id":        tokenID,
		"token":           tokenStr,
		"token_masked":    maskToken(tokenStr),
		"created_by":      tok.CreatedBy,
		"created_at":      tok.CreatedAt,
		"expires_at":      tok.ExpiresAt,
		"consumed_at":     nil,
		"consumed_by_agent_id": nil,
		"install_command": installCmd,
		"warnings":        warnings,
	})
}

// HandleListEnrollmentTokens handles GET /api/v1/agents/enrollment-tokens
func (h *AgentHandler) HandleListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	includeExpired := parseBoolQuery(r, "include_expired", false)
	includeConsumed := parseBoolQuery(r, "include_consumed", false)

	tokens, err := h.store.ListTokens(r.Context(), includeExpired, includeConsumed)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list tokens")
		return
	}
	if tokens == nil {
		tokens = []*agent.EnrollmentToken{}
	}

	type masked struct {
		TokenID           string     `json:"token_id"`
		TokenMasked       string     `json:"token_masked"`
		CreatedBy         string     `json:"created_by"`
		CreatedAt         time.Time  `json:"created_at"`
		ExpiresAt         time.Time  `json:"expires_at"`
		ConsumedAt        *time.Time `json:"consumed_at"`
		ConsumedByAgentID *string    `json:"consumed_by_agent_id"`
	}
	out := make([]masked, len(tokens))
	for i, t := range tokens {
		out[i] = masked{
			TokenID:           t.TokenID,
			TokenMasked:       maskToken(t.Token),
			CreatedBy:         t.CreatedBy,
			CreatedAt:         t.CreatedAt,
			ExpiresAt:         t.ExpiresAt,
			ConsumedAt:        t.ConsumedAt,
			ConsumedByAgentID: t.ConsumedByAgentID,
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

// HandleGetEnrollmentToken handles GET /api/v1/agents/enrollment-tokens/:token_id
func (h *AgentHandler) HandleGetEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	tokenID := r.PathValue("token_id")
	tok, err := h.store.GetTokenByID(r.Context(), tokenID)
	if errors.Is(err, agent.ErrTokenNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Token not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get token")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"token_id":             tok.TokenID,
		"token_masked":         maskToken(tok.Token),
		"created_by":           tok.CreatedBy,
		"created_at":           tok.CreatedAt,
		"expires_at":           tok.ExpiresAt,
		"consumed_at":          tok.ConsumedAt,
		"consumed_by_agent_id": tok.ConsumedByAgentID,
	})
}

// HandleDeleteEnrollmentToken handles DELETE /api/v1/agents/enrollment-tokens/:token_id
func (h *AgentHandler) HandleDeleteEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	tokenID := r.PathValue("token_id")
	err := h.store.DeleteToken(r.Context(), tokenID)
	if errors.Is(err, agent.ErrTokenNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Token not found")
		return
	}
	if errors.Is(err, agent.ErrTokenAlreadyConsumed) {
		WriteError(w, http.StatusConflict, "TOKEN_CONSUMED", "Cannot delete a consumed token")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Agent endpoints ─────────────────────────────────────────────────────────

// HandleListAgents handles GET /api/v1/agents
func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	connFilter := r.URL.Query().Get("connection_state")

	agents, err := h.store.List(r.Context(), statusFilter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list agents")
		return
	}
	if agents == nil {
		agents = []*agent.Agent{}
	}

	connected := h.connectedIDs()
	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		connState := "disconnected"
		if _, ok := connected[a.AgentID]; ok {
			connState = "connected"
		}
		if connFilter != "" && connFilter != connState {
			continue
		}
		out = append(out, agentToMap(a, connState))
	}

	WriteJSON(w, http.StatusOK, map[string]any{"agents": out})
}

// HandleGetAgent handles GET /api/v1/agents/:id
func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	a, err := h.store.Get(r.Context(), agentID)
	if errors.Is(err, agent.ErrAgentNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get agent")
		return
	}
	connected := h.connectedIDs()
	connState := "disconnected"
	if _, ok := connected[a.AgentID]; ok {
		connState = "connected"
	}
	WriteJSON(w, http.StatusOK, agentToMap(a, connState))
}

// HandleUpdateAgent handles PATCH /api/v1/agents/:id
func (h *AgentHandler) HandleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	var body struct {
		Label *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	if body.Label == nil {
		WriteError(w, http.StatusBadRequest, "MISSING_FIELD", "label is required")
		return
	}
	if len(*body.Label) > 64 {
		WriteError(w, http.StatusBadRequest, "LABEL_TOO_LONG", "label must be ≤ 64 characters")
		return
	}

	if err := h.store.UpdateLabel(r.Context(), agentID, *body.Label); err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
			return
		}
		if errors.Is(err, agent.ErrLabelTooLong) {
			WriteError(w, http.StatusBadRequest, "LABEL_TOO_LONG", "label must be ≤ 64 characters")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update label")
		return
	}

	a, err := h.store.Get(r.Context(), agentID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve updated agent")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.AgentUpdated, Data: map[string]any{
		"agent_id": agentID,
		"label":    *body.Label,
	}})

	connected := h.connectedIDs()
	connState := "disconnected"
	if _, ok := connected[a.AgentID]; ok {
		connState = "connected"
	}
	WriteJSON(w, http.StatusOK, agentToMap(a, connState))
}

// HandleRevokeAgent handles POST /api/v1/agents/:id/revoke
func (h *AgentHandler) HandleRevokeAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if err := h.store.Revoke(r.Context(), agentID, "admin"); err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke agent")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.AgentRevoked, Data: map[string]any{
		"agent_id": agentID,
	}})

	a, err := h.store.Get(r.Context(), agentID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve revoked agent")
		return
	}
	WriteJSON(w, http.StatusOK, agentToMap(a, "disconnected"))
}

// HandleDeleteAgent handles DELETE /api/v1/agents/:id
func (h *AgentHandler) HandleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if err := h.store.Delete(r.Context(), agentID); err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete agent")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.AgentDeleted, Data: map[string]any{
		"agent_id": agentID,
	}})
	w.WriteHeader(http.StatusNoContent)
}

// HandleGetAgentMetrics handles GET /api/v1/agents/metrics
func (h *AgentHandler) HandleGetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	active, revoked, err := h.store.CountByStatus(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch agent metrics")
		return
	}
	docker, swarm, kubernetes, err := h.store.CountByRuntime(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch agent metrics")
		return
	}
	connected := len(h.connectedIDs())
	total := active + revoked
	WriteJSON(w, http.StatusOK, map[string]any{
		"total": total,
		"by_status": map[string]int{
			"active":  active,
			"revoked": revoked,
		},
		"by_runtime": map[string]int{
			"docker":     docker,
			"swarm":      swarm,
			"kubernetes": kubernetes,
		},
		"by_connection_state": map[string]int{
			"connected":    connected,
			"disconnected": active - connected,
		},
		"total_events_per_second_observed_5m": 0,
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func agentToMap(a *agent.Agent, connectionState string) map[string]any {
	return map[string]any{
		"agent_id":         a.AgentID,
		"hostname":         a.Hostname,
		"label":            a.Label,
		"os_arch":          a.OSArch,
		"agent_version":    a.AgentVersion,
		"detected_runtime": a.DetectedRuntime,
		"status":           a.Status,
		"connection_state": connectionState,
		"last_seen_at":     a.LastSeenAt,
		"created_at":       a.CreatedAt,
		"revoked_at":       a.RevokedAt,
		"revoked_by":       a.RevokedBy,
	}
}

func maskToken(token string) string {
	if len(token) <= 14 {
		return token
	}
	return token[:14] + "...***"
}

func parseBoolQuery(r *http.Request, key string, def bool) bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
