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
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/kolapsis/maintenant/internal/certificate"
)

// CertificateHandler handles certificate monitor CRUD endpoints.
type CertificateHandler struct {
	svc *certificate.Service
}

// NewCertificateHandler creates a new certificate handler.
func NewCertificateHandler(svc *certificate.Service) *CertificateHandler {
	return &CertificateHandler{svc: svc}
}

// HandleList handles GET /api/v1/certificates
func (h *CertificateHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := certificate.ListCertificatesOpts{
		Status: r.URL.Query().Get("status"),
		Source: r.URL.Query().Get("source"),
	}

	monitors, err := h.svc.ListMonitors(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list certificates")
		return
	}

	if monitors == nil {
		monitors = []*certificate.CertMonitor{}
	}

	// Enrich with latest check data
	type monitorWithCheck struct {
		*certificate.CertMonitor
		LatestCheck *certificate.CertCheckResult `json:"latest_check,omitempty"`
	}

	enriched := make([]monitorWithCheck, len(monitors))
	for i, m := range monitors {
		enriched[i] = monitorWithCheck{CertMonitor: m}
		latest, err := h.svc.GetLatestCheckResult(r.Context(), m.ID)
		if err == nil && latest != nil {
			enriched[i].LatestCheck = latest
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"certificates": enriched,
		"total":        len(monitors),
	})
}

// HandleGet handles GET /api/v1/certificates/{id}
func (h *CertificateHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid certificate monitor ID")
		return
	}

	monitor, err := h.svc.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, certificate.ErrMonitorNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Certificate monitor not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get certificate monitor")
		return
	}

	response := map[string]interface{}{
		"certificate": monitor,
	}

	// Include latest check result with chain
	latest, err := h.svc.GetLatestCheckResult(r.Context(), id)
	if err == nil && latest != nil {
		chain, _ := h.svc.GetChainEntries(r.Context(), latest.ID)
		if chain == nil {
			chain = []*certificate.CertChainEntry{}
		}
		response["latest_check"] = map[string]interface{}{
			"id":                  latest.ID,
			"subject_cn":          latest.SubjectCN,
			"issuer_cn":           latest.IssuerCN,
			"issuer_org":          latest.IssuerOrg,
			"sans":                latest.SANs,
			"serial_number":       latest.SerialNumber,
			"signature_algorithm": latest.SignatureAlgorithm,
			"not_before":          latest.NotBefore,
			"not_after":           latest.NotAfter,
			"days_remaining":      latest.DaysRemaining(),
			"chain_valid":         latest.ChainValid,
			"chain_error":         latest.ChainError,
			"hostname_match":      latest.HostnameMatch,
			"error_message":       latest.ErrorMessage,
			"checked_at":          latest.CheckedAt,
			"chain":               chain,
		}
	}

	WriteJSON(w, http.StatusOK, response)
}

// HandleCreate handles POST /api/v1/certificates
func (h *CertificateHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input certificate.CreateCertificateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	monitor, checkResult, err := h.svc.CreateStandalone(r.Context(), input)
	if err != nil {
		if errors.Is(err, certificate.ErrAutoDetectedMonitor) {
			WriteError(w, http.StatusConflict, "ALREADY_AUTO_DETECTED",
				"hostname:port already monitored via endpoint auto-detection")
			return
		}
		if errors.Is(err, certificate.ErrDuplicateMonitor) {
			WriteError(w, http.StatusConflict, "DUPLICATE_MONITOR",
				"hostname:port already monitored")
			return
		}
		if errors.Is(err, certificate.ErrLimitReached) {
			WriteError(w, http.StatusForbidden, "QUOTA_EXCEEDED",
				"Community edition is limited to 5 certificate monitors. Upgrade to Pro for unlimited monitoring.")
			return
		}
		if errors.Is(err, certificate.ErrInvalidInput) {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		slog.Error("failed to create certificate monitor", "error", err, "hostname", input.Hostname, "port", input.Port)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create certificate monitor")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"certificate":  monitor,
		"latest_check": checkResult,
	})
}

// HandleUpdate handles PUT /api/v1/certificates/{id}
func (h *CertificateHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid certificate monitor ID")
		return
	}

	var input certificate.UpdateCertificateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	monitor, err := h.svc.UpdateMonitor(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, certificate.ErrMonitorNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Certificate monitor not found")
			return
		}
		if errors.Is(err, certificate.ErrInvalidInput) {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update certificate monitor")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"certificate": monitor,
	})
}

// HandleDelete handles DELETE /api/v1/certificates/{id}
func (h *CertificateHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid certificate monitor ID")
		return
	}

	if err := h.svc.DeleteMonitor(r.Context(), id); err != nil {
		if errors.Is(err, certificate.ErrMonitorNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Certificate monitor not found")
			return
		}
		if errors.Is(err, certificate.ErrCannotDeleteAuto) {
			WriteError(w, http.StatusBadRequest, "CANNOT_DELETE_AUTO",
				"Cannot delete auto-detected certificate monitors; remove the label instead")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete certificate monitor")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListChecks handles GET /api/v1/certificates/{id}/checks
func (h *CertificateHandler) HandleListChecks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid certificate monitor ID")
		return
	}

	// Verify monitor exists
	if _, err := h.svc.GetMonitor(r.Context(), id); err != nil {
		if errors.Is(err, certificate.ErrMonitorNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Certificate monitor not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get certificate monitor")
		return
	}

	opts := certificate.ListChecksOpts{}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		opts.Offset, _ = strconv.Atoi(v)
	}

	checks, total, err := h.svc.ListCheckResults(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list check results")
		return
	}

	if checks == nil {
		checks = []*certificate.CertCheckResult{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"monitor_id": id,
		"checks":     checks,
		"total":      total,
		"has_more":   opts.Offset+len(checks) < total,
	})
}
