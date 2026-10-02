// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
)

type handler struct {
	store  model.Store
	cfg    Config
	logger *slog.Logger
}

// NewHandler returns the anomaly detection API, routed under /api/v1/anomaly/.
func NewHandler(store model.Store, cfg Config, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{store: store, cfg: cfg, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/anomaly/summary", h.handleSummary)
	mux.HandleFunc("GET /api/v1/anomaly/settings", h.handleGetSettings)
	mux.HandleFunc("PUT /api/v1/anomaly/settings", h.handlePutSettings)
	mux.HandleFunc("GET /api/v1/anomaly/series", h.handleListSeries)
	mux.HandleFunc("GET /api/v1/anomaly/events", h.handleListEvents)
	// scope_id holds slashes, hence the trailing wildcards.
	mux.HandleFunc("GET /api/v1/anomaly/series/{scope_type}/{scope_id...}", h.handleSeriesDetail)
	mux.HandleFunc("GET /api/v1/anomaly/baseline/{scope_type}/{scope_id...}", h.handleBaseline)
	return mux
}

type scopeSummary struct {
	Learning        int     `json:"learning"`
	Ready           int     `json:"ready"`
	Relearning      int     `json:"relearning"`
	ActiveAnomalies int     `json:"active_anomalies"`
	Progress        float64 `json:"progress"`
	EarliestReadyAt *int64  `json:"earliest_ready_at"`
}

func (h *handler) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	states, err := h.store.ListSeriesStates(ctx, "")
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to list series")
		return
	}

	type acc struct {
		sum      scopeSummary
		progSum  float64
		progN    int
		earliest int64
	}
	by := map[string]*acc{
		model.ScopeTypeContainer: {},
		model.ScopeTypeHost:      {},
	}
	for _, st := range states {
		a := by[st.ScopeType]
		if a == nil {
			a = &acc{}
			by[st.ScopeType] = a
		}
		switch st.State {
		case model.StateLearning:
			a.sum.Learning++
		case model.StateReady:
			a.sum.Ready++
		case model.StateRelearning:
			a.sum.Relearning++
		}
		if st.State == model.StateLearning || st.State == model.StateRelearning {
			ready := EstimatedReadyAt(h.cfg, st.State, st.FirstSeenAt)
			if a.earliest == 0 || ready < a.earliest {
				a.earliest = ready
			}
		}
		a.progSum += st.Progress
		a.progN++
	}

	active := true
	out := map[string]scopeSummary{}
	for scopeType, a := range by {
		evs, err := h.store.ListAnomalyEvents(ctx, model.AnomalyEventFilter{ScopeType: scopeType, Tier: model.TierActive, Active: &active})
		if err != nil {
			v1.WriteStoreError(w, err, "Failed to list anomalies")
			return
		}
		s := a.sum
		s.ActiveAnomalies = len(evs)
		if a.progN > 0 {
			s.Progress = a.progSum / float64(a.progN)
		}
		if a.earliest > 0 {
			e := a.earliest
			s.EarliestReadyAt = &e
		}
		out[scopeType] = s
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

type seriesItem struct {
	ScopeType    string   `json:"scope_type"`
	ScopeID      string   `json:"scope_id"`
	Metric       string   `json:"metric"`
	Dimension    string   `json:"dimension"`
	NodeID       string   `json:"node_id"`
	State        string   `json:"state"`
	Progress     float64  `json:"progress"`
	Sensitivity  string   `json:"sensitivity"`
	Metrics      []string `json:"metrics"`
	CurrentScore float64  `json:"current_score"`
	ReadyAt      *int64   `json:"ready_at"`
	EstReadyAt   *int64   `json:"estimated_ready_at"`
}

func (h *handler) toItem(st *model.SeriesState) seriesItem {
	it := seriesItem{
		ScopeType:    st.ScopeType,
		ScopeID:      st.ScopeID,
		Metric:       st.Metric,
		Dimension:    st.Dimension,
		NodeID:       st.NodeID,
		State:        st.State,
		Progress:     st.Progress,
		Sensitivity:  st.Sensitivity,
		Metrics:      MetricsForScope(st.ScopeType),
		CurrentScore: st.CurrentScore,
		ReadyAt:      st.ReadyAt,
	}
	if st.State == model.StateLearning || st.State == model.StateRelearning {
		e := EstimatedReadyAt(h.cfg, st.State, st.FirstSeenAt)
		it.EstReadyAt = &e
	}
	return it
}

type settingsResponse struct {
	BucketPull    int    `json:"bucket_pull"`
	MinBucketPull int    `json:"min_bucket_pull"`
	MaxBucketPull int    `json:"max_bucket_pull"`
	MinSamples    int    `json:"min_samples"`
	AlertSeverity string `json:"alert_severity"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (h *handler) settingsPayload(s model.Settings) settingsResponse {
	return settingsResponse{
		BucketPull:    s.BucketPull,
		MinBucketPull: MinBucketPull,
		MaxBucketPull: MaxBucketPull,
		MinSamples:    h.cfg.MinSamples,
		AlertSeverity: h.cfg.Severity,
		UpdatedAt:     s.UpdatedAt,
	}
}

func (h *handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := LoadSettings(r.Context(), h.store)
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to read settings")
		return
	}
	v1.WriteJSON(w, http.StatusOK, h.settingsPayload(s))
}

func (h *handler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BucketPull *int `json:"bucket_pull"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON")
		return
	}
	if req.BucketPull == nil {
		v1.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "bucket_pull is required")
		return
	}
	if *req.BucketPull < MinBucketPull || *req.BucketPull > MaxBucketPull {
		v1.WriteError(w, http.StatusBadRequest, "INVALID_PARAM",
			fmt.Sprintf("bucket_pull must be between %d and %d", MinBucketPull, MaxBucketPull))
		return
	}

	s := model.Settings{BucketPull: *req.BucketPull, UpdatedAt: time.Now().Unix()}
	if err := h.store.UpdateSettings(r.Context(), s); err != nil {
		h.logger.ErrorContext(r.Context(), "anomaly: persist settings", "error", err)
		v1.WriteStoreError(w, err, "Failed to save settings")
		return
	}
	v1.WriteJSON(w, http.StatusOK, h.settingsPayload(s))
}

func (h *handler) handleListSeries(w http.ResponseWriter, r *http.Request) {
	states, err := h.store.ListSeriesStates(r.Context(), r.URL.Query().Get("scope_type"))
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to list series")
		return
	}
	items := make([]seriesItem, 0, len(states))
	for _, st := range states {
		items = append(items, h.toItem(st))
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"series": items})
}

func (h *handler) handleSeriesDetail(w http.ResponseWriter, r *http.Request) {
	scopeType := r.PathValue("scope_type")
	scopeID := r.PathValue("scope_id")
	if scopeType == "" || scopeID == "" {
		v1.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "scope_type and scope_id are required")
		return
	}
	states, err := h.store.ListSeriesStates(r.Context(), scopeType)
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to list series")
		return
	}
	items := make([]seriesItem, 0)
	for _, st := range states {
		if st.ScopeID == scopeID {
			items = append(items, h.toItem(st))
		}
	}
	if len(items) == 0 {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "scope not found")
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{
		"scope_type": scopeType,
		"scope_id":   scopeID,
		"metrics":    items,
	})
}

type bandPoint struct {
	Timestamp int64   `json:"timestamp"`
	Median    float64 `json:"median"`
	Lower     float64 `json:"lower"`
	Upper     float64 `json:"upper"`
}

// handleBaseline projects the seasonal band of a ready series onto the hours of [from,to], empty while it learns.
func (h *handler) handleBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scopeType := r.PathValue("scope_type")
	scopeID := r.PathValue("scope_id")
	metric := r.URL.Query().Get("metric")
	dimension := r.URL.Query().Get("dimension")
	if scopeType == "" || scopeID == "" || metric == "" {
		v1.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "scope_type, scope_id and metric are required")
		return
	}
	key := model.SeriesKey{ScopeType: scopeType, ScopeID: scopeID, Metric: metric, Dimension: dimension}

	state, err := h.store.GetSeriesState(ctx, key)
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to load series")
		return
	}
	points := []bandPoint{}
	if state != nil && state.State == model.StateReady {
		baselines, err := h.store.ListBaselines(ctx, key)
		if err != nil {
			v1.WriteStoreError(w, err, "Failed to load baseline")
			return
		}
		byBucket := make(map[int]model.Baseline, len(baselines))
		for _, b := range baselines {
			byBucket[b.Bucket] = b
		}

		now := time.Now().Unix()
		from := queryInt64(r, "from", now-24*3600)
		to := queryInt64(r, "to", now)
		loc := h.cfg.Location
		if loc == nil {
			loc = time.UTC
		}
		for ts := from - (from % 3600); ts <= to; ts += 3600 {
			b, ok := byBucket[TimeOfWeekBucket(time.Unix(ts, 0), loc)]
			if !ok {
				continue
			}
			lower, median, upper := BaselineBand(b, state.Sensitivity, metric)
			points = append(points, bandPoint{Timestamp: ts, Median: median, Lower: lower, Upper: upper})
		}
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"metric": metric, "dimension": dimension, "points": points})
}

type eventItem struct {
	ID             string  `json:"id"`
	ScopeType      string  `json:"scope_type"`
	ScopeID        string  `json:"scope_id"`
	Metric         string  `json:"metric"`
	Dimension      string  `json:"dimension"`
	NodeID         string  `json:"node_id"`
	Detector       string  `json:"detector"`
	Tier           string  `json:"tier"`
	StartedAt      int64   `json:"started_at"`
	EndedAt        *int64  `json:"ended_at"`
	PeakValue      float64 `json:"peak_value"`
	BaselineMedian float64 `json:"baseline_median"`
	PeakDeviation  float64 `json:"peak_deviation"`
	AlertID        string  `json:"alert_id,omitempty"`
	SuppressedBy   string  `json:"suppressed_by,omitempty"`
}

func (h *handler) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.AnomalyEventFilter{
		ScopeType: q.Get("scope_type"),
		ScopeID:   q.Get("scope_id"),
		NodeID:    q.Get("node_id"),
		Metric:    q.Get("metric"),
		Tier:      q.Get("tier"),
		Limit:     200,
	}
	if v := strings.TrimSpace(q.Get("active")); v != "" {
		b := v == "true" || v == "1"
		f.Active = &b
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			f.Limit = n
		}
	}
	events, err := h.store.ListAnomalyEvents(r.Context(), f)
	if err != nil {
		v1.WriteStoreError(w, err, "Failed to list events")
		return
	}
	items := make([]eventItem, 0, len(events))
	for _, e := range events {
		items = append(items, eventItem{
			ID: e.ID, ScopeType: e.ScopeType, ScopeID: e.ScopeID, Metric: e.Metric, Dimension: e.Dimension,
			NodeID: e.NodeID, Detector: e.Detector, Tier: e.Tier, StartedAt: e.StartedAt, EndedAt: e.EndedAt,
			PeakValue: e.PeakValue, BaselineMedian: e.BaselineMedian, PeakDeviation: e.PeakDeviation,
			AlertID: e.AlertID, SuppressedBy: e.SuppressedBy,
		})
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"events": items})
}

func queryInt64(r *http.Request, key string, fallback int64) int64 {
	if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
