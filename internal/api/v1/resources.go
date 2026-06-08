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
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/resource"
)

// ResourceHandler handles resource monitoring HTTP endpoints.
type ResourceHandler struct {
	service *resource.Service
	agents  AgentDirectory // optional — names hosts in the /resources/hosts list
}

// NewResourceHandler creates a new resource handler.
func NewResourceHandler(service *resource.Service) *ResourceHandler {
	return &ResourceHandler{service: service}
}

// SetAgentDirectory wires the agent directory so /resources/hosts can label
// remote hosts with their hostname/label. Nil-safe (falls back to ids).
func (h *ResourceHandler) SetAgentDirectory(d AgentDirectory) {
	h.agents = d
}

// parseHostFilter reads the ?agent_id= query param into a host filter:
//   - absent or empty  => nil  (caller decides default: "all" or "local")
//   - "local"          => pointer to "" (the local server host, NULL agent_id)
//   - "<agent_id>"     => pointer to that agent id
func parseHostFilter(r *http.Request) *string {
	v := r.URL.Query().Get("agent_id")
	if v == "" {
		return nil
	}
	if v == "local" {
		empty := ""
		return &empty
	}
	return &v
}

// hostMatches reports whether a snapshot owned by snapAgent belongs to the host
// described by filter (nil = all, "" = local/NULL, id = that agent).
func hostMatches(snapAgent *string, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "" {
		return snapAgent == nil
	}
	return snapAgent != nil && *snapAgent == *filter
}

// HandleGetCurrent handles GET /api/v1/containers/{id}/resources/current.
func (h *ResourceHandler) HandleGetCurrent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid container ID")
		return
	}

	snap := h.service.GetCurrentSnapshot(id)
	if snap == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "No resource data available for container")
		return
	}

	memPercent := 0.0
	if snap.MemLimit > 0 {
		memPercent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":      snap.ContainerID,
		"cpu_percent":       snap.CPUPercent,
		"mem_used":          snap.MemUsed,
		"mem_limit":         snap.MemLimit,
		"mem_percent":       memPercent,
		"net_rx_bytes":      snap.NetRxBytes,
		"net_tx_bytes":      snap.NetTxBytes,
		"block_read_bytes":  snap.BlockReadBytes,
		"block_write_bytes": snap.BlockWriteBytes,
		"timestamp":         snap.Timestamp,
	})
}

// HandleGetSummary handles GET /api/v1/resources/summary[?agent_id=local|<id>].
// Without agent_id it reports the local server host (preserving prior behaviour).
func (h *ResourceHandler) HandleGetSummary(w http.ResponseWriter, r *http.Request) {
	filter := parseHostFilter(r)
	// Summary is always scoped to one host; default to the local server.
	if filter == nil {
		empty := ""
		filter = &empty
	}

	// Net rates and container count for the selected host only.
	var totalNetRxRate, totalNetTxRate int64
	containerCount := 0
	for _, snap := range h.service.GetAllLatestSnapshots() {
		if !hostMatches(snap.AgentID, filter) {
			continue
		}
		totalNetRxRate += snap.NetRxBytes
		totalNetTxRate += snap.NetTxBytes
		containerCount++
	}

	sample := h.service.HostStatForAgent(*filter)
	available := sample != nil

	var cpuPercent, memPercent, diskPercent float64
	var memUsed, memLimit int64
	var diskTotal, diskUsed uint64
	if available {
		cpuPercent = sample.CPUPercent
		memUsed, memLimit = sample.MemUsed, sample.MemTotal
		if memLimit > 0 {
			memPercent = float64(memUsed) / float64(memLimit) * 100.0
		}
		diskTotal, diskUsed = sample.DiskTotal, sample.DiskUsed
		if diskTotal > 0 {
			diskPercent = float64(diskUsed) / float64(diskTotal) * 100.0
		}
	}

	// CPU core count is only known for the local server host.
	cpuCount := 0
	if *filter == "" {
		cpuCount = runtime.NumCPU()
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"agent_id":          *filter,
		"available":         available,
		"total_cpu_percent": cpuPercent,
		"cpu_count":         cpuCount,
		"total_mem_used":    memUsed,
		"total_mem_limit":   memLimit,
		"total_mem_percent": memPercent,
		"total_net_rx_rate": totalNetRxRate,
		"total_net_tx_rate": totalNetTxRate,
		"container_count":   containerCount,
		"disk_total":        diskTotal,
		"disk_used":         diskUsed,
		"disk_percent":      diskPercent,
		"timestamp":         time.Now(),
	})
}

// HandleGetHosts handles GET /api/v1/resources/hosts. It lists every known host
// (the local server plus each remote agent) with its current CPU/mem/disk and
// running-container count, so the UI can offer a host selector.
func (h *ResourceHandler) HandleGetHosts(w http.ResponseWriter, r *http.Request) {
	// Running-container count per host, from the in-memory latest snapshots.
	counts := map[string]int{}
	for _, snap := range h.service.GetAllLatestSnapshots() {
		key := ""
		if snap.AgentID != nil {
			key = *snap.AgentID
		}
		counts[key]++
	}

	var names map[string]AgentName
	if h.agents != nil {
		if m, err := h.agents.AgentNames(r.Context()); err == nil {
			names = m
		}
	}

	type hostDTO struct {
		AgentID        string  `json:"agent_id"` // "" for the local server
		Hostname       string  `json:"hostname"`
		Label          string  `json:"label"`
		IsLocal        bool    `json:"is_local"`
		Available      bool    `json:"available"`
		CPUPercent     float64 `json:"cpu_percent"`
		MemUsed        int64   `json:"mem_used"`
		MemTotal       int64   `json:"mem_total"`
		MemPercent     float64 `json:"mem_percent"`
		DiskTotal      uint64  `json:"disk_total"`
		DiskUsed       uint64  `json:"disk_used"`
		DiskPercent    float64 `json:"disk_percent"`
		ContainerCount int     `json:"container_count"`
	}

	now := time.Now()
	build := func(agentID string, sample *resource.HostSample) hostDTO {
		dto := hostDTO{
			AgentID:        agentID,
			IsLocal:        agentID == "",
			ContainerCount: counts[agentID],
		}
		if agentID == "" {
			dto.Hostname = "Local"
			dto.Label = "Local"
		} else if n, ok := names[agentID]; ok {
			dto.Hostname = n.Hostname
			dto.Label = n.Label
		}
		if resource.IsHostSampleFresh(sample, now) {
			dto.Available = true
			dto.CPUPercent = sample.CPUPercent
			dto.MemUsed, dto.MemTotal = sample.MemUsed, sample.MemTotal
			if sample.MemTotal > 0 {
				dto.MemPercent = float64(sample.MemUsed) / float64(sample.MemTotal) * 100.0
			}
			dto.DiskTotal, dto.DiskUsed = sample.DiskTotal, sample.DiskUsed
			if sample.DiskTotal > 0 {
				dto.DiskPercent = float64(sample.DiskUsed) / float64(sample.DiskTotal) * 100.0
			}
		}
		return dto
	}

	hosts := []hostDTO{}
	seen := map[string]bool{}
	for _, sample := range h.service.ListHostStats() {
		hosts = append(hosts, build(sample.AgentID, sample))
		seen[sample.AgentID] = true
	}
	// Include enrolled agents that have not reported a host sample yet so the
	// selector still lists them (as unavailable).
	for agentID := range names {
		if !seen[agentID] {
			hosts = append(hosts, build(agentID, nil))
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

// HandleGetHistory handles GET /api/v1/containers/{id}/resources/history.
func (h *ResourceHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid container ID")
		return
	}

	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	switch timeRange {
	case "1h", "6h", "24h", "7d":
	default:
		WriteError(w, http.StatusBadRequest, "INVALID_RANGE", "Range must be 1h, 6h, 24h, or 7d")
		return
	}

	snaps, granularity, err := h.service.GetHistory(r.Context(), id, timeRange)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch resource history")
		return
	}

	points := make([]map[string]interface{}, 0, len(snaps))
	for _, s := range snaps {
		points = append(points, map[string]interface{}{
			"timestamp":         s.Timestamp,
			"cpu_percent":       s.CPUPercent,
			"mem_used":          s.MemUsed,
			"mem_limit":         s.MemLimit,
			"net_rx_bytes":      s.NetRxBytes,
			"net_tx_bytes":      s.NetTxBytes,
			"block_read_bytes":  s.BlockReadBytes,
			"block_write_bytes": s.BlockWriteBytes,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id": id,
		"range":        timeRange,
		"granularity":  granularity,
		"points":       points,
	})
}

// HandleGetAlertConfig handles GET /api/v1/containers/{id}/resources/alerts.
func (h *ResourceHandler) HandleGetAlertConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid container ID")
		return
	}

	cfg, err := h.service.GetAlertConfig(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch alert config")
		return
	}

	if cfg == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"container_id":    id,
			"cpu_threshold":   90.0,
			"mem_threshold":   90.0,
			"enabled":         false,
			"alert_state":     "normal",
			"last_alerted_at": nil,
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":    cfg.ContainerID,
		"cpu_threshold":   cfg.CPUThreshold,
		"mem_threshold":   cfg.MemThreshold,
		"enabled":         cfg.Enabled,
		"alert_state":     cfg.AlertState,
		"last_alerted_at": cfg.LastAlertedAt,
	})
}

// HandleUpsertAlertConfig handles PUT /api/v1/containers/{id}/resources/alerts.
func (h *ResourceHandler) HandleUpsertAlertConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid container ID")
		return
	}

	var input struct {
		CPUThreshold float64 `json:"cpu_threshold"`
		MemThreshold float64 `json:"mem_threshold"`
		Enabled      bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if input.CPUThreshold < 1 || input.CPUThreshold > 1000 {
		WriteError(w, http.StatusBadRequest, "INVALID_THRESHOLD", "CPU threshold must be between 1 and 1000")
		return
	}
	if input.MemThreshold < 1 || input.MemThreshold > 100 {
		WriteError(w, http.StatusBadRequest, "INVALID_THRESHOLD", "Memory threshold must be between 1 and 100")
		return
	}

	// Get existing config to preserve state fields.
	existing, _ := h.service.GetAlertConfig(r.Context(), id)
	cfg := &resource.ResourceAlertConfig{
		ContainerID:  id,
		CPUThreshold: input.CPUThreshold,
		MemThreshold: input.MemThreshold,
		Enabled:      input.Enabled,
		AlertState:   resource.AlertStateNormal,
	}
	if existing != nil {
		cfg.AlertState = existing.AlertState
		cfg.CPUConsecutiveBreaches = existing.CPUConsecutiveBreaches
		cfg.MemConsecutiveBreaches = existing.MemConsecutiveBreaches
		cfg.LastAlertedAt = existing.LastAlertedAt
	}

	if err := h.service.UpsertAlertConfig(r.Context(), cfg); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save alert config")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":    cfg.ContainerID,
		"cpu_threshold":   cfg.CPUThreshold,
		"mem_threshold":   cfg.MemThreshold,
		"enabled":         cfg.Enabled,
		"alert_state":     cfg.AlertState,
		"last_alerted_at": cfg.LastAlertedAt,
	})
}
