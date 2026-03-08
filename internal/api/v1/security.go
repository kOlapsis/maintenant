// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"net/http"
	"strconv"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/security"
)

// SecurityHandler handles security insight HTTP endpoints.
type SecurityHandler struct {
	securitySvc  *security.Service
	containerSvc *container.Service
}

// NewSecurityHandler creates a new security handler.
func NewSecurityHandler(secSvc *security.Service, containerSvc *container.Service) *SecurityHandler {
	return &SecurityHandler{securitySvc: secSvc, containerSvc: containerSvc}
}

// HandleListInsights handles GET /api/v1/security/insights.
func (h *SecurityHandler) HandleListInsights(w http.ResponseWriter, r *http.Request) {
	all := h.securitySvc.GetAllInsights()

	containers, err := h.containerSvc.ListContainers(r.Context(), container.ListContainersOpts{IncludeArchived: false})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list containers")
		return
	}

	summary := h.securitySvc.GetSummary(len(containers))

	WriteJSON(w, http.StatusOK, map[string]any{
		"containers": all,
		"summary":    summary,
	})
}

// HandleGetContainerInsights handles GET /api/v1/security/insights/{container_id}.
func (h *SecurityHandler) HandleGetContainerInsights(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("container_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	c, err := h.containerSvc.GetContainer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	ci := h.securitySvc.GetContainerInsights(id)
	ci.ContainerName = c.Name

	WriteJSON(w, http.StatusOK, ci)
}

// HandleGetSummary handles GET /api/v1/security/summary.
func (h *SecurityHandler) HandleGetSummary(w http.ResponseWriter, r *http.Request) {
	containers, err := h.containerSvc.ListContainers(r.Context(), container.ListContainersOpts{IncludeArchived: false})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list containers")
		return
	}

	summary := h.securitySvc.GetSummary(len(containers))
	WriteJSON(w, http.StatusOK, summary)
}
