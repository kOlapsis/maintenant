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
	"net/http"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/update"
)

// CVEHandler handles CVE-related HTTP endpoints.
type CVEHandler struct {
	store update.UpdateStore
}

// NewCVEHandler creates a new CVE handler.
func NewCVEHandler(store update.UpdateStore) *CVEHandler {
	return &CVEHandler{store: store}
}

// HandleListCVEs handles GET /api/v1/cve.
func (h *CVEHandler) HandleListCVEs(w http.ResponseWriter, r *http.Request) {
	if !extension.Allows(extension.CapCVEEnrichment) {
		refuseCapability(w, extension.CapCVEEnrichment)
		return
	}

	opts := update.ListCVEsOpts{
		Severity:    r.URL.Query().Get("severity"),
		ContainerID: r.URL.Query().Get("container_id"),
	}

	cves, err := h.store.ListAllActiveCVEs(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list CVEs")
		return
	}

	// Group by CVE ID for the response format
	type affectedContainer struct {
		ContainerID string `json:"container_id"`
		FixedIn     string `json:"fixed_in,omitempty"`
	}

	cveMap := make(map[string]map[string]interface{})
	bySeverity := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}

	for _, c := range cves {
		bySeverity[string(c.Severity)]++
		if existing, ok := cveMap[c.CVEID]; ok {
			affected := existing["affected_containers"].([]affectedContainer)
			existing["affected_containers"] = append(affected, affectedContainer{
				ContainerID: c.ContainerID,
				FixedIn:     c.FixedIn,
			})
		} else {
			cveMap[c.CVEID] = map[string]interface{}{
				"cve_id":              c.CVEID,
				"cvss_score":          c.CVSSScore,
				"severity":            string(c.Severity),
				"summary":             c.Summary,
				"first_detected_at":   c.FirstDetectedAt,
				"affected_containers": []affectedContainer{{ContainerID: c.ContainerID, FixedIn: c.FixedIn}},
			}
		}
	}

	cveList := make([]map[string]interface{}, 0, len(cveMap))
	for _, v := range cveMap {
		cveList = append(cveList, v)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"cves":        cveList,
		"total":       len(cves),
		"by_severity": bySeverity,
	})
}

// HandleGetContainerCVEs handles GET /api/v1/cve/{container_id}.
func (h *CVEHandler) HandleGetContainerCVEs(w http.ResponseWriter, r *http.Request) {
	if !extension.Allows(extension.CapCVEEnrichment) {
		refuseCapability(w, extension.CapCVEEnrichment)
		return
	}

	containerID := r.PathValue("container_id")
	if containerID == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Missing container_id")
		return
	}

	cves, err := h.store.ListContainerCVEs(r.Context(), containerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container CVEs")
		return
	}

	cveMaps := make([]map[string]interface{}, 0, len(cves))
	for _, c := range cves {
		cveMaps = append(cveMaps, map[string]interface{}{
			"cve_id":            c.CVEID,
			"cvss_score":        c.CVSSScore,
			"severity":          string(c.Severity),
			"summary":           c.Summary,
			"fixed_in":          c.FixedIn,
			"first_detected_at": c.FirstDetectedAt,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id": containerID,
		"cves":         cveMaps,
	})
}
