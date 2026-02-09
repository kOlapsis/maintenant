package v1

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/kolapsis/pulseboard/internal/status"
)

// StatusAdminHandler handles admin endpoints for the public status page.
type StatusAdminHandler struct {
	components status.ComponentStore
	statusSvc  *status.Service
	broker     *SSEBroker
}

// NewStatusAdminHandler creates a new status admin handler.
func NewStatusAdminHandler(
	components status.ComponentStore,
	statusSvc *status.Service,
	broker *SSEBroker,
) *StatusAdminHandler {
	return &StatusAdminHandler{
		components: components,
		statusSvc:  statusSvc,
		broker:     broker,
	}
}

// --- Component Groups ---

func (h *StatusAdminHandler) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.components.ListGroups(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, groups)
}

func (h *StatusAdminHandler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g status.ComponentGroup
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if g.Name == "" {
		WriteError(w, http.StatusBadRequest, "validation", "Name is required")
		return
	}
	if _, err := h.components.CreateGroup(r.Context(), &g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, g)
}

func (h *StatusAdminHandler) HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	existing, err := h.components.GetGroup(r.Context(), id)
	if err != nil || existing == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Group not found")
		return
	}
	var g status.ComponentGroup
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	g.ID = id
	if g.Name == "" {
		g.Name = existing.Name
	}
	if err := h.components.UpdateGroup(r.Context(), &g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, g)
}

func (h *StatusAdminHandler) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	if err := h.components.DeleteGroup(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Status Components ---

func (h *StatusAdminHandler) HandleListComponents(w http.ResponseWriter, r *http.Request) {
	components, err := h.components.ListComponents(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// Derive effective status for each
	for i := range components {
		components[i].DerivedStatus = h.statusSvc.DeriveComponentStatus(r.Context(), &components[i])
		if components[i].StatusOverride != nil {
			components[i].EffectiveStatus = *components[i].StatusOverride
		} else {
			components[i].EffectiveStatus = components[i].DerivedStatus
		}
	}
	WriteJSON(w, http.StatusOK, components)
}

func (h *StatusAdminHandler) HandleCreateComponent(w http.ResponseWriter, r *http.Request) {
	var c status.StatusComponent
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if c.MonitorType == "" || c.MonitorID == 0 || c.DisplayName == "" {
		WriteError(w, http.StatusBadRequest, "validation", "monitor_type, monitor_id, and display_name are required")
		return
	}
	// Check for duplicate
	existing, _ := h.components.GetComponentByMonitor(r.Context(), c.MonitorType, c.MonitorID)
	if existing != nil {
		WriteError(w, http.StatusConflict, "conflict", "Component already exists for this monitor")
		return
	}
	if _, err := h.components.CreateComponent(r.Context(), &c); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.EffectiveStatus = h.statusSvc.DeriveComponentStatus(r.Context(), &c)
	WriteJSON(w, http.StatusCreated, c)
}

func (h *StatusAdminHandler) HandleUpdateComponent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid component ID")
		return
	}
	existing, err := h.components.GetComponent(r.Context(), id)
	if err != nil || existing == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Component not found")
		return
	}
	var upd struct {
		DisplayName    *string `json:"display_name"`
		GroupID        *int64  `json:"group_id"`
		DisplayOrder   *int    `json:"display_order"`
		Visible        *bool   `json:"visible"`
		StatusOverride *string `json:"status_override"`
		AutoIncident   *bool   `json:"auto_incident"`
	}
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if upd.DisplayName != nil {
		existing.DisplayName = *upd.DisplayName
	}
	if upd.GroupID != nil {
		existing.GroupID = upd.GroupID
	}
	if upd.DisplayOrder != nil {
		existing.DisplayOrder = *upd.DisplayOrder
	}
	if upd.Visible != nil {
		existing.Visible = *upd.Visible
	}
	if upd.StatusOverride != nil {
		if *upd.StatusOverride == "" {
			existing.StatusOverride = nil
		} else {
			existing.StatusOverride = upd.StatusOverride
		}
	}
	if upd.AutoIncident != nil {
		existing.AutoIncident = *upd.AutoIncident
	}
	if err := h.components.UpdateComponent(r.Context(), existing); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	existing.EffectiveStatus = h.statusSvc.DeriveComponentStatus(r.Context(), existing)
	WriteJSON(w, http.StatusOK, existing)
}

func (h *StatusAdminHandler) HandleDeleteComponent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid component ID")
		return
	}
	if err := h.components.DeleteComponent(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
