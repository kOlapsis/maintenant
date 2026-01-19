package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"github.com/kolapsis/pulseboard/internal/resource"
)

// ResourceHandler handles resource monitoring HTTP endpoints.
type ResourceHandler struct {
	service *resource.Service
}

// NewResourceHandler creates a new resource handler.
func NewResourceHandler(service *resource.Service) *ResourceHandler {
	return &ResourceHandler{service: service}
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

// HandleGetSummary handles GET /api/v1/resources/summary.
func (h *ResourceHandler) HandleGetSummary(w http.ResponseWriter, r *http.Request) {
	all := h.service.GetAllLatestSnapshots()

	var totalCPU float64
	var totalMemUsed, totalMemLimit int64
	var totalNetRxRate, totalNetTxRate int64

	for _, snap := range all {
		totalCPU += snap.CPUPercent
		totalMemUsed += snap.MemUsed
		totalMemLimit += snap.MemLimit
		totalNetRxRate += snap.NetRxBytes
		totalNetTxRate += snap.NetTxBytes
	}

	totalMemPercent := 0.0
	if totalMemLimit > 0 {
		totalMemPercent = float64(totalMemUsed) / float64(totalMemLimit) * 100.0
	}

	// Host disk usage (root filesystem)
	var diskTotal, diskUsed uint64
	var diskPercent float64
	var fs syscall.Statfs_t
	if err := syscall.Statfs("/", &fs); err == nil {
		diskTotal = fs.Blocks * uint64(fs.Bsize)
		diskFree := fs.Bavail * uint64(fs.Bsize)
		diskUsed = diskTotal - diskFree
		if diskTotal > 0 {
			diskPercent = float64(diskUsed) / float64(diskTotal) * 100.0
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"total_cpu_percent": totalCPU,
		"total_mem_used":    totalMemUsed,
		"total_mem_limit":   totalMemLimit,
		"total_mem_percent": totalMemPercent,
		"total_net_rx_rate": totalNetRxRate,
		"total_net_tx_rate": totalNetTxRate,
		"container_count":   len(all),
		"disk_total":        diskTotal,
		"disk_used":         diskUsed,
		"disk_percent":      diskPercent,
		"timestamp":         time.Now(),
	})
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
