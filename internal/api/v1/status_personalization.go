package v1

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/kolapsis/maintenant/internal/status"
)

type PersonalizationHandler struct {
	svc *status.PersonalizationService
}

func NewPersonalizationHandler(svc *status.PersonalizationService) *PersonalizationHandler {
	return &PersonalizationHandler{svc: svc}
}

// --- Settings ---

func (h *PersonalizationHandler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.GetSettings(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, settingsToResponse(s))
}

func (h *PersonalizationHandler) HandlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Colors   struct {
			Bg                string `json:"bg"`
			Surface           string `json:"surface"`
			Border            string `json:"border"`
			Text              string `json:"text"`
			Accent            string `json:"accent"`
			StatusOperational string `json:"status_operational"`
			StatusDegraded    string `json:"status_degraded"`
			StatusPartial     string `json:"status_partial"`
			StatusMajor       string `json:"status_major"`
		} `json:"colors"`
		Announcement struct {
			Enabled   bool   `json:"enabled"`
			MessageMD string `json:"message_md"`
			URL       string `json:"url"`
		} `json:"announcement"`
		FooterTextMD string `json:"footer_text_md"`
		Locale       string `json:"locale"`
		Timezone     string `json:"timezone"`
		DateFormat   string `json:"date_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}

	in := status.Settings{
		Title:    req.Title,
		Subtitle: req.Subtitle,
		Colors: status.Palette{
			Background:          req.Colors.Bg,
			Surface:             req.Colors.Surface,
			Border:              req.Colors.Border,
			Text:                req.Colors.Text,
			Accent:              req.Colors.Accent,
			StatusOperational:   req.Colors.StatusOperational,
			StatusDegraded:      req.Colors.StatusDegraded,
			StatusPartialOutage: req.Colors.StatusPartial,
			StatusMajorOutage:   req.Colors.StatusMajor,
		},
		Announcement: status.Announcement{
			Enabled:   req.Announcement.Enabled,
			MessageMD: req.Announcement.MessageMD,
			URL:       req.Announcement.URL,
		},
		FooterTextMD: req.FooterTextMD,
		Locale:       req.Locale,
		Timezone:     req.Timezone,
		DateFormat:   req.DateFormat,
	}

	out, warnings, err := h.svc.UpdateSettings(r.Context(), in)
	if err != nil {
		code, msg := mapSettingsError(err)
		WriteError(w, code, "validation_error", msg)
		return
	}

	resp := settingsToResponse(out)
	if len(warnings) > 0 {
		type withWarnings struct {
			Version      int64                      `json:"version"`
			Title        string                     `json:"title"`
			Subtitle     string                     `json:"subtitle"`
			Colors       settingsColorsResp         `json:"colors"`
			Announcement settingsAnnouncementResp   `json:"announcement"`
			FooterTextMD string                     `json:"footer_text_md"`
			FooterTextHTML string                   `json:"footer_text_html"`
			Locale       string                     `json:"locale"`
			Timezone     string                     `json:"timezone"`
			DateFormat   string                     `json:"date_format"`
			UpdatedAt    string                     `json:"updated_at"`
			Warnings     map[string][]status.ContrastWarning `json:"warnings"`
		}
		WriteJSON(w, http.StatusOK, withWarnings{
			Version:      out.Version,
			Title:        out.Title,
			Subtitle:     out.Subtitle,
			Colors:       resp.Colors,
			Announcement: resp.Announcement,
			FooterTextMD:   out.FooterTextMD,
			FooterTextHTML: out.FooterTextHTML,
			Locale:       out.Locale,
			Timezone:     out.Timezone,
			DateFormat:   out.DateFormat,
			UpdatedAt:    out.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			Warnings:     map[string][]status.ContrastWarning{"contrast": warnings},
		})
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

// --- Assets ---

func (h *PersonalizationHandler) HandlePutAsset(w http.ResponseWriter, r *http.Request) {
	roleStr := r.PathValue("role")
	role := status.AssetRole(roleStr)
	switch role {
	case status.AssetRoleLogo, status.AssetRoleFavicon, status.AssetRoleHero:
	default:
		WriteError(w, http.StatusBadRequest, "validation_error", "unknown asset role")
		return
	}

	cap := status.AssetSizeCap(role)
	r.Body = http.MaxBytesReader(w, r.Body, cap+1024) // +1024 for form overhead

	if err := r.ParseMultipartForm(cap); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			WriteError(w, http.StatusBadRequest, "payload_too_large", "asset exceeds size limit")
			return
		}
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "file part is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, cap+1))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to read file")
		return
	}
	if int64(len(data)) > cap {
		WriteError(w, http.StatusBadRequest, "payload_too_large", "asset exceeds size limit")
		return
	}

	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mime, err := status.DetectAssetMIME(role, head)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "unsupported_mime", "MIME type not allowed for this role")
		return
	}

	altText := r.FormValue("alt_text")
	if err := h.svc.PutAsset(r.Context(), role, mime, data, altText); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"role":       string(role),
		"mime":       mime,
		"byte_size":  len(data),
		"alt_text":   altText,
	})
}

func (h *PersonalizationHandler) HandleGetAsset(w http.ResponseWriter, r *http.Request) {
	role := status.AssetRole(r.PathValue("role"))
	asset, err := h.svc.GetAsset(r.Context(), role)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if asset == nil {
		WriteError(w, http.StatusNotFound, "not_found", "no asset for this role")
		return
	}
	w.Header().Set("Content-Type", asset.MIME)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(asset.Bytes)
}

func (h *PersonalizationHandler) HandleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	role := status.AssetRole(r.PathValue("role"))
	if err := h.svc.DeleteAsset(r.Context(), role); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Footer links ---

func (h *PersonalizationHandler) HandleListFooterLinks(w http.ResponseWriter, r *http.Request) {
	links, err := h.svc.ListFooterLinks(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"items": footerLinksToResponse(links)})
}

func (h *PersonalizationHandler) HandleCreateFooterLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	link, err := h.svc.CreateFooterLink(r.Context(), req.Label, req.URL)
	if err != nil {
		code, msg := mapLinkError(err)
		WriteError(w, code, "validation_error", msg)
		return
	}
	WriteJSON(w, http.StatusCreated, footerLinkToResponse(link))
}

func (h *PersonalizationHandler) HandleUpdateFooterLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid id")
		return
	}
	var req struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	link, err := h.svc.UpdateFooterLink(r.Context(), id, req.Label, req.URL)
	if err != nil {
		if errors.Is(err, status.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "footer link not found")
			return
		}
		code, msg := mapLinkError(err)
		WriteError(w, code, "validation_error", msg)
		return
	}
	WriteJSON(w, http.StatusOK, footerLinkToResponse(link))
}

func (h *PersonalizationHandler) HandleDeleteFooterLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid id")
		return
	}
	if err := h.svc.DeleteFooterLink(r.Context(), id); err != nil {
		if errors.Is(err, status.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "footer link not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PersonalizationHandler) HandleReorderFooterLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	links, err := h.svc.ReorderFooterLinks(r.Context(), req.IDs)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"items": footerLinksToResponse(links)})
}

// --- FAQ ---

func (h *PersonalizationHandler) HandleListFAQ(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListFAQItems(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"items": faqItemsToResponse(items)})
}

func (h *PersonalizationHandler) HandleCreateFAQItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question"`
		AnswerMD string `json:"answer_md"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	item, err := h.svc.CreateFAQItem(r.Context(), req.Question, req.AnswerMD)
	if err != nil {
		code, msg := mapFAQError(err)
		WriteError(w, code, "validation_error", msg)
		return
	}
	WriteJSON(w, http.StatusCreated, faqItemToResponse(item))
}

func (h *PersonalizationHandler) HandleUpdateFAQItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid id")
		return
	}
	var req struct {
		Question string `json:"question"`
		AnswerMD string `json:"answer_md"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	item, err := h.svc.UpdateFAQItem(r.Context(), id, req.Question, req.AnswerMD)
	if err != nil {
		if errors.Is(err, status.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "FAQ item not found")
			return
		}
		code, msg := mapFAQError(err)
		WriteError(w, code, "validation_error", msg)
		return
	}
	WriteJSON(w, http.StatusOK, faqItemToResponse(item))
}

func (h *PersonalizationHandler) HandleDeleteFAQItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid id")
		return
	}
	if err := h.svc.DeleteFAQItem(r.Context(), id); err != nil {
		if errors.Is(err, status.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "FAQ item not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PersonalizationHandler) HandleReorderFAQ(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid JSON")
		return
	}
	items, err := h.svc.ReorderFAQItems(r.Context(), req.IDs)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"items": faqItemsToResponse(items)})
}

// --- Response helpers ---

type settingsColorsResp struct {
	Bg                string `json:"bg"`
	Surface           string `json:"surface"`
	Border            string `json:"border"`
	Text              string `json:"text"`
	Accent            string `json:"accent"`
	StatusOperational string `json:"status_operational"`
	StatusDegraded    string `json:"status_degraded"`
	StatusPartial     string `json:"status_partial"`
	StatusMajor       string `json:"status_major"`
}

type settingsAnnouncementResp struct {
	Enabled     bool   `json:"enabled"`
	MessageMD   string `json:"message_md"`
	MessageHTML string `json:"message_html"`
	URL         string `json:"url"`
}

type settingsResp struct {
	Version        int64                    `json:"version"`
	Title          string                   `json:"title"`
	Subtitle       string                   `json:"subtitle"`
	Colors         settingsColorsResp       `json:"colors"`
	Announcement   settingsAnnouncementResp `json:"announcement"`
	FooterTextMD   string                   `json:"footer_text_md"`
	FooterTextHTML string                   `json:"footer_text_html"`
	Locale         string                   `json:"locale"`
	Timezone       string                   `json:"timezone"`
	DateFormat     string                   `json:"date_format"`
	UpdatedAt      string                   `json:"updated_at"`
}

func settingsToResponse(s status.Settings) settingsResp {
	return settingsResp{
		Version:  s.Version,
		Title:    s.Title,
		Subtitle: s.Subtitle,
		Colors: settingsColorsResp{
			Bg:                s.Colors.Background,
			Surface:           s.Colors.Surface,
			Border:            s.Colors.Border,
			Text:              s.Colors.Text,
			Accent:            s.Colors.Accent,
			StatusOperational: s.Colors.StatusOperational,
			StatusDegraded:    s.Colors.StatusDegraded,
			StatusPartial:     s.Colors.StatusPartialOutage,
			StatusMajor:       s.Colors.StatusMajorOutage,
		},
		Announcement: settingsAnnouncementResp{
			Enabled:     s.Announcement.Enabled,
			MessageMD:   s.Announcement.MessageMD,
			MessageHTML: s.Announcement.MessageHTML,
			URL:         s.Announcement.URL,
		},
		FooterTextMD:   s.FooterTextMD,
		FooterTextHTML: s.FooterTextHTML,
		Locale:         s.Locale,
		Timezone:       s.Timezone,
		DateFormat:     s.DateFormat,
		UpdatedAt:      s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

type footerLinkResp struct {
	ID        int64  `json:"id"`
	Position  int    `json:"position"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func footerLinkToResponse(l status.FooterLink) footerLinkResp {
	return footerLinkResp{
		ID:        l.ID,
		Position:  l.Position,
		Label:     l.Label,
		URL:       l.URL,
		CreatedAt: l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: l.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func footerLinksToResponse(links []status.FooterLink) []footerLinkResp {
	out := make([]footerLinkResp, len(links))
	for i, l := range links {
		out[i] = footerLinkToResponse(l)
	}
	return out
}

type faqItemResp struct {
	ID         int64  `json:"id"`
	Position   int    `json:"position"`
	Question   string `json:"question"`
	AnswerMD   string `json:"answer_md"`
	AnswerHTML string `json:"answer_html"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func faqItemToResponse(item status.FAQItem) faqItemResp {
	return faqItemResp{
		ID:         item.ID,
		Position:   item.Position,
		Question:   item.Question,
		AnswerMD:   item.AnswerMD,
		AnswerHTML: item.AnswerHTML,
		CreatedAt:  item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func faqItemsToResponse(items []status.FAQItem) []faqItemResp {
	out := make([]faqItemResp, len(items))
	for i, item := range items {
		out[i] = faqItemToResponse(item)
	}
	return out
}

func assetDataURL(a *status.Asset) string {
	if a == nil {
		return ""
	}
	return "data:" + a.MIME + ";base64," + base64.StdEncoding.EncodeToString(a.Bytes)
}

func mapSettingsError(err error) (int, string) {
	switch {
	case errors.Is(err, status.ErrInvalidHex):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrInvalidScheme):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrInvalidLocale):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrInvalidDateFormat):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrInvalidTimezone):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrFieldTooLong):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func mapLinkError(err error) (int, string) {
	switch {
	case errors.Is(err, status.ErrInvalidScheme):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, status.ErrFieldTooLong):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

func mapFAQError(err error) (int, string) {
	switch {
	case errors.Is(err, status.ErrFieldTooLong):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
