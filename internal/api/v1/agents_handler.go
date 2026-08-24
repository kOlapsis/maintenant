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
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
)

type AgentHandler struct {
	store          *store.AgentStore
	sessions       AgentSessions
	broker         *SSEBroker
	logger         *slog.Logger
	grpcPublicURL  string
	grpcListen     string
	staleThreshold time.Duration
}

func NewAgentHandler(
	store *store.AgentStore,
	sessions AgentSessions,
	broker *SSEBroker,
	logger *slog.Logger,
	grpcPublicURL string,
	grpcListen string,
	staleThreshold time.Duration,
) *AgentHandler {
	return &AgentHandler{
		store:          store,
		sessions:       sessions,
		broker:         broker,
		logger:         logger,
		grpcPublicURL:  grpcPublicURL,
		grpcListen:     grpcListen,
		staleThreshold: staleThreshold,
	}
}

func (h *AgentHandler) HandleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	// Refuse up front when the host cap is already reached, so the operator sees
	// it before running the install command. The gRPC RegisterAgent path stays
	// the authoritative barrier. The local runtime is never counted.
	if limit := extension.Limit(extension.ResourceAgentHosts); limit >= 0 {
		active, _, err := h.store.CountByStatus(r.Context())
		if err != nil {
			h.logger.Error("count agents for host limit", "err", err)
			WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to check host limit")
			return
		}
		if active >= limit {
			writeQuotaRefusal(w, http.StatusConflict, "HOST_LIMIT_REACHED", extension.ResourceAgentHosts)
			return
		}
	}

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

	// This response is the only place the cleartext ever exists: what gets
	// persisted is the hash and a display prefix.
	tokenStr, tokenHash, tokenID, tokenPrefix, err := agent.NewToken()
	if err != nil {
		WriteStoreError(w, err, "Failed to generate token")
		return
	}

	now := time.Now().UTC()
	tok := &agent.EnrollmentToken{
		TokenID:     tokenID,
		TokenHash:   tokenHash,
		TokenPrefix: tokenPrefix,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := h.store.InsertToken(r.Context(), tok); err != nil {
		h.logger.Error("insert enrollment token", "err", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create token")
		return
	}

	publicURL, warnings := agentserver.ResolvePublicURL(r, agentserver.PublicURLConfig{
		Explicit:   h.grpcPublicURL,
		ListenAddr: h.grpcListen,
	})

	WriteJSON(w, http.StatusCreated, map[string]any{
		"token_id":             tokenID,
		"token":                tokenStr,
		"token_masked":         tok.Masked(),
		"created_at":           tok.CreatedAt,
		"expires_at":           tok.ExpiresAt,
		"consumed_at":          nil,
		"consumed_by_agent_id": nil,
		"install_templates":    buildInstallTemplates(publicURL, tokenStr),
		"warnings":             warnings,
	})
}

func buildInstallTemplates(serverURL, token string) map[string]string {
	return map[string]string{
		"standalone":     buildInstallStandalone(serverURL, token),
		"docker_run":     buildInstallDockerRun(serverURL, token),
		"docker_compose": buildInstallDockerCompose(serverURL, token),
		"kubernetes":     buildInstallKubernetes(serverURL, token),
	}
}

func buildInstallStandalone(serverURL, token string) string {
	return "curl -fsSL https://install.maintenant.dev | sudo bash -s -- \\\n" +
		"  --mode=agent \\\n" +
		"  --server=" + serverURL + " \\\n" +
		"  --enrollment-token=" + token
}

func buildInstallDockerRun(serverURL, token string) string {
	return "docker run -d \\\n" +
		"  --name maintenant-agent \\\n" +
		"  --restart unless-stopped \\\n" +
		"  -v /var/run/docker.sock:/var/run/docker.sock:ro \\\n" +
		"  -v /proc:/host/proc:ro \\\n" +
		"  -v maintenant-agent-data:/var/lib/maintenant \\\n" +
		"  ghcr.io/kolapsis/maintenant:latest \\\n" +
		"  --mode=agent \\\n" +
		"  --server=" + serverURL + " \\\n" +
		"  --enrollment-token=" + token
}

func buildInstallDockerCompose(serverURL, token string) string {
	return "services:\n" +
		"  maintenant-agent:\n" +
		"    image: ghcr.io/kolapsis/maintenant:latest\n" +
		"    restart: unless-stopped\n" +
		"    volumes:\n" +
		"      - /var/run/docker.sock:/var/run/docker.sock:ro\n" +
		"      - /proc:/host/proc:ro\n" +
		"      - maintenant-agent-data:/var/lib/maintenant\n" +
		"    command:\n" +
		"      - --mode=agent\n" +
		"      - --server=" + serverURL + "\n" +
		"      - --enrollment-token=" + token + "\n" +
		"\n" +
		"volumes:\n" +
		"  maintenant-agent-data:\n"
}

func buildInstallKubernetes(serverURL, token string) string {
	return "apiVersion: v1\n" +
		"kind: Namespace\n" +
		"metadata:\n" +
		"  name: maintenant\n" +
		"---\n" +
		"apiVersion: v1\n" +
		"kind: Secret\n" +
		"metadata:\n" +
		"  name: maintenant-agent-enrollment\n" +
		"  namespace: maintenant\n" +
		"stringData:\n" +
		"  token: " + token + "\n" +
		"---\n" +
		"apiVersion: v1\n" +
		"kind: ServiceAccount\n" +
		"metadata:\n" +
		"  name: maintenant-agent\n" +
		"  namespace: maintenant\n" +
		"---\n" +
		"apiVersion: rbac.authorization.k8s.io/v1\n" +
		"kind: ClusterRole\n" +
		"metadata:\n" +
		"  name: maintenant-agent\n" +
		"rules:\n" +
		"  - apiGroups: [\"\"]\n" +
		"    resources: [pods, nodes, services, events]\n" +
		"    verbs: [get, list, watch]\n" +
		"---\n" +
		"apiVersion: rbac.authorization.k8s.io/v1\n" +
		"kind: ClusterRoleBinding\n" +
		"metadata:\n" +
		"  name: maintenant-agent\n" +
		"roleRef:\n" +
		"  apiGroup: rbac.authorization.k8s.io\n" +
		"  kind: ClusterRole\n" +
		"  name: maintenant-agent\n" +
		"subjects:\n" +
		"  - kind: ServiceAccount\n" +
		"    name: maintenant-agent\n" +
		"    namespace: maintenant\n" +
		"---\n" +
		"apiVersion: apps/v1\n" +
		"kind: DaemonSet\n" +
		"metadata:\n" +
		"  name: maintenant-agent\n" +
		"  namespace: maintenant\n" +
		"spec:\n" +
		"  selector:\n" +
		"    matchLabels: { app: maintenant-agent }\n" +
		"  template:\n" +
		"    metadata:\n" +
		"      labels: { app: maintenant-agent }\n" +
		"    spec:\n" +
		"      serviceAccountName: maintenant-agent\n" +
		"      containers:\n" +
		"        - name: agent\n" +
		"          image: ghcr.io/kolapsis/maintenant:latest\n" +
		"          args:\n" +
		"            - --mode=agent\n" +
		"            - --server=" + serverURL + "\n" +
		"            - --enrollment-token=$(MAINTENANT_ENROLLMENT_TOKEN)\n" +
		"            - --runtime=kubernetes\n" +
		"          env:\n" +
		"            - name: MAINTENANT_ENROLLMENT_TOKEN\n" +
		"              valueFrom:\n" +
		"                secretKeyRef:\n" +
		"                  name: maintenant-agent-enrollment\n" +
		"                  key: token\n" +
		"            - name: MAINTENANT_LABEL\n" +
		"              valueFrom:\n" +
		"                fieldRef: { fieldPath: spec.nodeName }\n" +
		"          volumeMounts:\n" +
		"            - { name: identity, mountPath: /var/lib/maintenant }\n" +
		"      volumes:\n" +
		"        - name: identity\n" +
		"          hostPath:\n" +
		"            path: /var/lib/maintenant-agent\n" +
		"            type: DirectoryOrCreate\n"
}

func (h *AgentHandler) HandleListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	includeExpired := parseBoolQuery(r, "include_expired", false)
	includeConsumed := parseBoolQuery(r, "include_consumed", false)

	tokens, err := h.store.ListTokens(r.Context(), includeExpired, includeConsumed)
	if err != nil {
		WriteStoreError(w, err, "Failed to list tokens")
		return
	}
	if tokens == nil {
		tokens = []*agent.EnrollmentToken{}
	}

	type masked struct {
		TokenID           string     `json:"token_id"`
		TokenMasked       string     `json:"token_masked"`
		CreatedAt         time.Time  `json:"created_at"`
		ExpiresAt         time.Time  `json:"expires_at"`
		ConsumedAt        *time.Time `json:"consumed_at"`
		ConsumedByAgentID *string    `json:"consumed_by_agent_id"`
	}
	out := make([]masked, len(tokens))
	for i, t := range tokens {
		out[i] = masked{
			TokenID:           t.TokenID,
			TokenMasked:       t.Masked(),
			CreatedAt:         t.CreatedAt,
			ExpiresAt:         t.ExpiresAt,
			ConsumedAt:        t.ConsumedAt,
			ConsumedByAgentID: t.ConsumedByAgentID,
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (h *AgentHandler) HandleGetEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	tokenID := r.PathValue("token_id")
	tok, err := h.store.GetTokenByID(r.Context(), tokenID)
	if errors.Is(err, agent.ErrTokenNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Token not found")
		return
	}
	if err != nil {
		WriteStoreError(w, err, "Failed to get token")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"token_id":             tok.TokenID,
		"token_masked":         tok.Masked(),
		"created_at":           tok.CreatedAt,
		"expires_at":           tok.ExpiresAt,
		"consumed_at":          tok.ConsumedAt,
		"consumed_by_agent_id": tok.ConsumedByAgentID,
	})
}

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
		WriteStoreError(w, err, "Failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	connFilter := r.URL.Query().Get("connection_state")

	agents, err := h.store.List(r.Context(), statusFilter)
	if err != nil {
		WriteStoreError(w, err, "Failed to list agents")
		return
	}
	if agents == nil {
		agents = []*agent.Agent{}
	}

	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		connState := h.resolveConnectionState(a)
		if connFilter != "" && connFilter != connState {
			continue
		}
		out = append(out, agentToMap(a, connState))
	}

	WriteJSON(w, http.StatusOK, map[string]any{"agents": out})
}

func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	a, err := h.store.Get(r.Context(), agentID)
	if errors.Is(err, agent.ErrAgentNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
		return
	}
	if err != nil {
		WriteStoreError(w, err, "Failed to get agent")
		return
	}
	WriteJSON(w, http.StatusOK, agentToMap(a, h.resolveConnectionState(a)))
}

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
		WriteStoreError(w, err, "Failed to retrieve updated agent")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.AgentUpdated, Data: map[string]any{
		"agent_id": agentID,
		"label":    *body.Label,
	}})

	WriteJSON(w, http.StatusOK, agentToMap(a, h.resolveConnectionState(a)))
}

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

	if h.sessions != nil {
		h.sessions.Close(agentID, "revoked")
	}

	h.logger.Info("agent.revoked", "agent_id", agentID, "revoked_by", "admin")
	h.broker.Broadcast(SSEEvent{Type: event.AgentRevoked, Data: map[string]any{
		"agent_id": agentID,
	}})

	a, err := h.store.Get(r.Context(), agentID)
	if err != nil {
		WriteStoreError(w, err, "Failed to retrieve revoked agent")
		return
	}
	WriteJSON(w, http.StatusOK, agentToMap(a, "disconnected"))
}

func (h *AgentHandler) HandleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	if h.sessions != nil {
		h.sessions.Close(agentID, "deleted")
	}

	if err := h.store.Delete(r.Context(), agentID); err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Agent not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete agent")
		return
	}

	h.logger.Info("agent.deleted", "agent_id", agentID)
	h.broker.Broadcast(SSEEvent{Type: event.AgentDeleted, Data: map[string]any{
		"agent_id": agentID,
	}})
	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentHandler) HandleGetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	active, revoked, err := h.store.CountByStatus(r.Context())
	if err != nil {
		WriteStoreError(w, err, "Failed to fetch agent metrics")
		return
	}
	docker, swarmCount, kubernetes, err := h.store.CountByRuntime(r.Context())
	if err != nil {
		WriteStoreError(w, err, "Failed to fetch agent metrics")
		return
	}

	connected := 0
	var epsRate float64
	if svc, ok := h.sessions.(interface {
		ListConnected() []string
		EventsPerSecond5m() float64
	}); ok {
		connected = len(svc.ListConnected())
		epsRate = svc.EventsPerSecond5m()
	}

	total := active + revoked
	WriteJSON(w, http.StatusOK, map[string]any{
		"total": total,
		"by_status": map[string]int{
			"active":  active,
			"revoked": revoked,
		},
		"by_runtime": map[string]int{
			"docker":     docker,
			"swarm":      swarmCount,
			"kubernetes": kubernetes,
		},
		"by_connection_state": map[string]int{
			"connected":    connected,
			"disconnected": active - connected,
		},
		"total_events_per_second_observed_5m": epsRate,
	})
}

// resolveConnectionState returns "connected" if the agent has an active stream
// or was last seen within staleThreshold, otherwise "disconnected".
func (h *AgentHandler) resolveConnectionState(a *agent.Agent) string {
	if h.sessions != nil && h.sessions.IsConnected(a.AgentID) {
		return "connected"
	}
	if a.LastSeenAt != nil && time.Since(*a.LastSeenAt) < h.staleThreshold {
		return "connected"
	}
	return "disconnected"
}

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
