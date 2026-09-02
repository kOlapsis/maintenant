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

package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// ContainerNameLister lists container names in a K8s workload pod spec.
type ContainerNameLister interface {
	ListContainerNames(ctx context.Context, externalID string) ([]string, error)
}

// SecurityInsightProvider provides security insight counts for containers.
type SecurityInsightProvider interface {
	InsightCount(containerID string) (int, string)
}

// RuntimeChecker reports whether the container runtime is currently connected.
type RuntimeChecker interface {
	IsConnected() bool
}

// AgentName holds the display identity of a remote agent.
type AgentName struct {
	Hostname string
	Label    string
}

// AgentDirectory resolves agent_id → display identity so container responses can
// show which agent a remote container belongs to.
type AgentDirectory interface {
	AgentNames(ctx context.Context) (map[string]AgentName, error)
}

// enrichAgentFields tags a response row with the remote agent that reported it,
// for the multi-host list views. Local rows (the server's own runtime) get no
// agent fields, so single-host installs and the "local" scope stay clean.
func enrichAgentFields(m map[string]interface{}, agentID string, names map[string]AgentName) {
	if agentID == "" || agentID == uid.LocalAgent {
		return
	}
	m["agent_id"] = agentID
	if n, ok := names[agentID]; ok {
		if n.Hostname != "" {
			m["agent_hostname"] = n.Hostname
		}
		if n.Label != "" {
			m["agent_label"] = n.Label
		}
	}
}

// ContainerHandler handles container-related HTTP endpoints.
type ContainerHandler struct {
	service          *container.Service
	uptime           *container.UptimeCalculator
	logFetcher       LogFetcher
	containerLister  ContainerNameLister
	securityProvider SecurityInsightProvider
	runtimeChecker   RuntimeChecker
	agentDirectory   AgentDirectory
	sessions         agentLiveness
	logRequester     AgentLogRequester
}

// SetLogRequester wires the command channel used to read logs of containers that
// live on a remote agent's host, which the server's own runtime cannot see.
func (h *ContainerHandler) SetLogRequester(lr AgentLogRequester) {
	h.logRequester = lr
}

// agentLabel resolves a human-friendly name for an agent, for error messages.
func (h *ContainerHandler) agentLabel(ctx context.Context, agentID string) string {
	return resolveAgentLabel(ctx, h.agentDirectory, agentID)
}

// NewContainerHandler creates a new container handler.
func NewContainerHandler(service *container.Service, uptime *container.UptimeCalculator) *ContainerHandler {
	return &ContainerHandler{service: service, uptime: uptime}
}

// SetRuntimeChecker injects the runtime availability checker.
func (h *ContainerHandler) SetRuntimeChecker(rc RuntimeChecker) {
	h.runtimeChecker = rc
}

// SetSecurityProvider sets the security insight provider for enriching container responses.
func (h *ContainerHandler) SetSecurityProvider(sp SecurityInsightProvider) {
	h.securityProvider = sp
}

// SetAgentSessions wires live agent-stream state so containers reported by a
// disconnected remote agent can be flagged stale (their last-known state is no
// longer live). The runtimeChecker still governs the local runtime separately.
func (h *ContainerHandler) SetAgentSessions(s agentLiveness) {
	h.sessions = s
}

// agentOffline reports whether a remote agent has no live stream. The local
// runtime (empty/LocalAgent) is governed by the runtime checker, not sessions.
func (h *ContainerHandler) agentOffline(agentID string) bool {
	return agentID != "" && agentID != uid.LocalAgent && h.sessions != nil && !h.sessions.IsConnected(agentID)
}

// SetAgentDirectory sets the agent directory for enriching remote containers with
// their originating agent's hostname/label.
func (h *ContainerHandler) SetAgentDirectory(ad AgentDirectory) {
	h.agentDirectory = ad
}

// SetLogFetcher sets the log fetcher for the logs endpoint.
func (h *ContainerHandler) SetLogFetcher(lf LogFetcher) {
	h.logFetcher = lf
}

// SetContainerNameLister sets the K8s container name lister for detail endpoints.
func (h *ContainerHandler) SetContainerNameLister(cl ContainerNameLister) {
	h.containerLister = cl
}

// HandleList handles GET /api/v1/containers.
func (h *ContainerHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := container.ListContainersOpts{}

	if r.URL.Query().Get("archived") == "true" {
		opts.IncludeArchived = true
	}
	if g := r.URL.Query().Get("group"); g != "" {
		opts.GroupFilter = g
	}
	if s := r.URL.Query().Get("state"); s != "" {
		opts.StateFilter = s
	}
	if a := r.URL.Query().Get("agent_id"); a != "" {
		// The container store matches agent_id verbatim, so resolve the "local"
		// alias to the sentinel id here (unlike the cert/endpoint/heartbeat
		// stores, which special-case "local" internally).
		if a == "local" {
			a = uid.LocalAgent
		}
		opts.AgentFilter = &a
	}

	groups, total, archivedCount, err := h.service.ListContainersGrouped(r.Context(), opts)
	if err != nil {
		WriteStoreError(w, err, "Failed to list containers")
		return
	}

	if groups == nil {
		groups = []*container.ContainerGroup{}
	}

	// Enrich with security insight counts and remote-agent identity if available.
	type enrichedContainer struct {
		*container.Container
		SecurityInsightCount    int     `json:"security_insight_count"`
		SecurityHighestSeverity *string `json:"security_highest_severity"`
		AgentHostname           *string `json:"agent_hostname,omitempty"`
		AgentLabel              *string `json:"agent_label,omitempty"`
		Stale                   *bool   `json:"stale,omitempty"`
		AgentOffline            *bool   `json:"agent_offline,omitempty"`
	}
	type enrichedGroup struct {
		Name       string              `json:"name"`
		Source     string              `json:"source"`
		Containers []enrichedContainer `json:"containers"`
	}

	// Resolve agent identities once for the whole response.
	var agentNames map[string]AgentName
	if h.agentDirectory != nil {
		if names, err := h.agentDirectory.AgentNames(r.Context()); err == nil {
			agentNames = names
		}
	}

	enrichedGroups := make([]enrichedGroup, 0, len(groups))
	for _, g := range groups {
		eg := enrichedGroup{Name: g.Name, Source: g.Source}
		eg.Containers = make([]enrichedContainer, 0, len(g.Containers))
		for _, c := range g.Containers {
			ec := enrichedContainer{Container: c}
			if h.securityProvider != nil {
				count, sev := h.securityProvider.InsightCount(c.ID)
				ec.SecurityInsightCount = count
				if sev != "" {
					ec.SecurityHighestSeverity = &sev
				}
			}
			if c.AgentID != "" && c.AgentID != uid.LocalAgent && agentNames != nil {
				if an, ok := agentNames[c.AgentID]; ok {
					hostname, label := an.Hostname, an.Label
					ec.AgentHostname = &hostname
					if label != "" {
						ec.AgentLabel = &label
					}
				}
			}
			if h.agentOffline(c.AgentID) {
				t := true
				ec.Stale = &t
				ec.AgentOffline = &t
			}
			eg.Containers = append(eg.Containers, ec)
		}
		enrichedGroups = append(enrichedGroups, eg)
	}

	resp := map[string]interface{}{
		"groups":         enrichedGroups,
		"total":          total,
		"archived_count": archivedCount,
	}
	if h.runtimeChecker != nil && !h.runtimeChecker.IsConnected() {
		resp["stale"] = true
	}
	WriteJSON(w, http.StatusOK, resp)
}

// HandleGet handles GET /api/v1/containers/{id}.
func (h *ContainerHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	c, err := h.service.GetContainer(r.Context(), id)
	if err != nil {
		WriteStoreError(w, err, "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	// Build detail response with uptime
	detail := map[string]interface{}{
		"id":                   c.ID,
		"external_id":          c.ExternalID,
		"name":                 c.Name,
		"image":                c.Image,
		"state":                c.State,
		"health_status":        c.HealthStatus,
		"has_health_check":     c.HasHealthCheck,
		"orchestration_group":  c.OrchestrationGroup,
		"orchestration_unit":   c.OrchestrationUnit,
		"custom_group":         c.CustomGroup,
		"is_ignored":           c.IsIgnored,
		"alert_severity":       c.AlertSeverity,
		"restart_threshold":    c.RestartThreshold,
		"alert_channels":       c.AlertChannels,
		"archived":             c.Archived,
		"first_seen_at":        c.FirstSeenAt,
		"last_state_change_at": c.LastStateChangeAt,
		"archived_at":          c.ArchivedAt,
		"runtime_type":         c.RuntimeType,
		"error_detail":         c.ErrorDetail,
		"controller_kind":      c.ControllerKind,
		"namespace":            c.Namespace,
		"pod_count":            c.PodCount,
		"ready_count":          c.ReadyCount,
	}

	// Add uptime if calculator is available
	if h.uptime != nil {
		uptimeResult, err := h.uptime.Calculate(r.Context(), c.ID, false)
		if err == nil && uptimeResult != nil {
			detail["uptime"] = uptimeResult
		}
	}

	// For K8s workloads, include container names from pod spec
	if c.RuntimeType == "kubernetes" && h.containerLister != nil {
		names, err := h.containerLister.ListContainerNames(r.Context(), c.ExternalID)
		if err == nil && len(names) > 0 {
			detail["container_names"] = names
		}
	}

	// Which host reported this container — the detail panel names it, so a fleet
	// with a dozen identically-named containers stays readable.
	if h.agentDirectory != nil {
		if names, err := h.agentDirectory.AgentNames(r.Context()); err == nil {
			enrichAgentFields(detail, c.AgentID, names)
		}
	}

	if h.runtimeChecker != nil && !h.runtimeChecker.IsConnected() {
		detail["stale"] = true
	}
	if h.agentOffline(c.AgentID) {
		detail["stale"] = true
		detail["agent_offline"] = true
	}

	WriteJSON(w, http.StatusOK, detail)
}

// HandleTransitions handles GET /api/v1/containers/{id}/transitions.
func (h *ContainerHandler) HandleTransitions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	opts := container.ListTransitionsOpts{
		Limit: 50,
	}

	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err == nil {
			opts.Since = &t
		}
	} else {
		since := time.Now().Add(-24 * time.Hour)
		opts.Since = &since
	}

	if u := r.URL.Query().Get("until"); u != "" {
		t, err := time.Parse(time.RFC3339, u)
		if err == nil {
			opts.Until = &t
		}
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	transitions, total, err := h.service.ListTransitions(r.Context(), id, opts)
	if err != nil {
		WriteStoreError(w, err, "Failed to list transitions")
		return
	}

	resp := map[string]interface{}{
		"container_id": id,
		"transitions":  transitions,
		"total":        total,
		"has_more":     opts.Offset+len(transitions) < total,
	}
	if h.runtimeChecker != nil && !h.runtimeChecker.IsConnected() {
		resp["stale"] = true
	}
	WriteJSON(w, http.StatusOK, resp)
}

// HandleDelete handles DELETE /api/v1/containers/{id}.
// Only allows deletion of non-running containers.
func (h *ContainerHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	c, err := h.service.GetContainer(r.Context(), id)
	if err != nil {
		WriteStoreError(w, err, "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	if c.State == "running" {
		WriteError(w, http.StatusConflict, "CONTAINER_RUNNING", "Cannot delete a running container")
		return
	}

	if err := h.service.DeleteContainer(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete container")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// LogFetcher abstracts Docker log retrieval for the API layer.
type LogFetcher interface {
	FetchLogs(ctx context.Context, containerID string, lines int, timestamps bool) ([]string, error)
}

// HandleLogs handles GET /api/v1/containers/{id}/logs.
func (h *ContainerHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	c, err := h.service.GetContainer(r.Context(), id)
	if err != nil {
		WriteStoreError(w, err, "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
			if lines > 500 {
				lines = 500
			}
		}
	}

	timestamps := r.URL.Query().Get("timestamps") == "true"

	var logLines []string

	// A container on a remote agent is invisible to our own runtime; only the
	// agent that reported it can read its logs.
	if isRemoteAgent(c.AgentID) {
		if h.logRequester == nil {
			WriteError(w, http.StatusBadGateway, "RUNTIME_UNAVAILABLE",
				"Multi-host agent support is not enabled on this server.")
			return
		}
		remote, err := fetchRemoteLogs(r.Context(), h.logRequester, c.AgentID, c.ExternalID, lines, timestamps)
		if err != nil {
			writeRemoteLogsError(w, h.agentLabel(r.Context(), c.AgentID), err)
			return
		}
		logLines = remote
	} else {
		if h.logFetcher == nil {
			WriteError(w, http.StatusBadGateway, "RUNTIME_UNAVAILABLE",
				"Cannot connect to container runtime for log retrieval.")
			return
		}
		local, err := h.logFetcher.FetchLogs(r.Context(), c.ExternalID, lines, timestamps)
		if err != nil {
			WriteError(w, http.StatusBadGateway, "LOGS_UNAVAILABLE", "Cannot retrieve logs from Docker")
			return
		}
		logLines = local
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":   c.ID,
		"container_name": c.Name,
		"lines":          logLines,
		"total_lines":    len(logLines),
		"truncated":      len(logLines) >= lines,
	})
}
