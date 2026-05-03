package status

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kolapsis/maintenant/internal/extension"
)

type PersonalizationPublicHandler struct {
	svc    *PersonalizationService
	logger *slog.Logger
}

func NewPersonalizationPublicHandler(svc *PersonalizationService, logger *slog.Logger) *PersonalizationPublicHandler {
	return &PersonalizationPublicHandler{svc: svc, logger: logger}
}

type PublicSettingsResponse struct {
	Version  int64                 `json:"version"`
	Title    string                `json:"title"`
	Subtitle string                `json:"subtitle"`
	Colors   PublicColorsResp      `json:"colors"`
	Assets   PublicAssetsResp      `json:"assets"`
	Announcement PublicAnnouncement `json:"announcement"`
	Footer   PublicFooterResp      `json:"footer"`
	Locale   string                `json:"locale"`
	Timezone string                `json:"timezone"`
	DateFormat string              `json:"date_format"`
}

type PublicColorsResp struct {
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

type PublicAssetsResp struct {
	Logo    *PublicAssetItem `json:"logo,omitempty"`
	Favicon *PublicAssetItem `json:"favicon,omitempty"`
	Hero    *PublicAssetItem `json:"hero,omitempty"`
}

type PublicAssetItem struct {
	DataURL string `json:"data_url"`
	AltText string `json:"alt_text,omitempty"`
}

type PublicAnnouncement struct {
	Enabled bool   `json:"enabled"`
	HTML    string `json:"html"`
	URL     string `json:"url"`
}

type PublicFooterResp struct {
	HTML      string             `json:"html"`
	Links     []PublicFooterLink `json:"links"`
	PoweredBy PublicPoweredBy    `json:"powered_by"`
}

type PublicFooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type PublicPoweredBy struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// GetVersion returns the current personalization settings version (0 on error).
func (h *PersonalizationPublicHandler) GetVersion(r *http.Request) int64 {
	s, err := h.svc.GetSettings(r.Context())
	if err != nil {
		return 0
	}
	return s.Version
}

func (h *PersonalizationPublicHandler) HandleSettingsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	settings, err := h.svc.GetSettings(ctx)
	if err != nil {
		h.logger.Error("failed to get personalization settings", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	isEnterprise := extension.CurrentEdition() == extension.Enterprise

	if !isEnterprise {
		settings = DefaultSettings()
	}

	etag := fmt.Sprintf(`"v%d"`, settings.Version)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	resp := PublicSettingsResponse{
		Version:  settings.Version,
		Title:    settings.Title,
		Subtitle: settings.Subtitle,
		Colors: PublicColorsResp{
			Bg:                settings.Colors.Background,
			Surface:           settings.Colors.Surface,
			Border:            settings.Colors.Border,
			Text:              settings.Colors.Text,
			Accent:            settings.Colors.Accent,
			StatusOperational: settings.Colors.StatusOperational,
			StatusDegraded:    settings.Colors.StatusDegraded,
			StatusPartial:     settings.Colors.StatusPartialOutage,
			StatusMajor:       settings.Colors.StatusMajorOutage,
		},
		Announcement: PublicAnnouncement{
			Enabled: settings.Announcement.Enabled,
			HTML:    settings.Announcement.MessageHTML,
			URL:     settings.Announcement.URL,
		},
		Footer: PublicFooterResp{
			HTML:  settings.FooterTextHTML,
			Links: []PublicFooterLink{},
			PoweredBy: PublicPoweredBy{
				Label: "Powered by Maintenant",
				URL:   "https://maintenant.dev",
			},
		},
		Locale:     settings.Locale,
		Timezone:   settings.Timezone,
		DateFormat: settings.DateFormat,
	}

	if isEnterprise {
		for _, role := range []AssetRole{AssetRoleLogo, AssetRoleFavicon, AssetRoleHero} {
			asset, err := h.svc.GetAsset(ctx, role)
			if err != nil {
				h.logger.Error("failed to get asset", "role", role, "error", err)
				continue
			}
			if asset == nil {
				continue
			}
			item := &PublicAssetItem{
				DataURL: "data:" + asset.MIME + ";base64," + base64.StdEncoding.EncodeToString(asset.Bytes),
				AltText: asset.AltText,
			}
			switch role {
			case AssetRoleLogo:
				resp.Assets.Logo = item
			case AssetRoleFavicon:
				resp.Assets.Favicon = item
			case AssetRoleHero:
				resp.Assets.Hero = item
			}
		}

		links, err := h.svc.ListFooterLinks(ctx)
		if err != nil {
			h.logger.Error("failed to list footer links", "error", err)
		} else {
			for _, l := range links {
				resp.Footer.Links = append(resp.Footer.Links, PublicFooterLink{Label: l.Label, URL: l.URL})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	w.Header().Set("ETag", etag)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(resp)
}
