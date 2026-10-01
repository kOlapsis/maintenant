// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
)

var hexColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

type cachedPayload struct {
	data    status.Settings
	builtAt time.Time
}

type PersonalizationService struct {
	store  status.PersonalizationStore
	logger *slog.Logger

	mu    sync.RWMutex
	cache *cachedPayload
}

func NewPersonalizationService(store status.PersonalizationStore, logger *slog.Logger) *PersonalizationService {
	return &PersonalizationService{store: store, logger: logger}
}

func (svc *PersonalizationService) GetSettings(ctx context.Context) (status.Settings, error) {
	svc.mu.RLock()
	c := svc.cache
	svc.mu.RUnlock()
	if c != nil && time.Since(c.builtAt) < 30*time.Second {
		return c.data, nil
	}
	return svc.store.GetSettings(ctx)
}

func (svc *PersonalizationService) UpdateSettings(ctx context.Context, in status.Settings) (status.Settings, []status.ContrastWarning, error) {
	if err := svc.validateSettings(in); err != nil {
		return status.Settings{}, nil, err
	}

	var err error
	in.Announcement.MessageHTML, err = RenderAnnouncement(in.Announcement.MessageMD)
	if err != nil {
		return status.Settings{}, nil, fmt.Errorf("render announcement: %w", err)
	}
	in.FooterTextHTML, err = RenderFooter(in.FooterTextMD)
	if err != nil {
		return status.Settings{}, nil, fmt.Errorf("render footer: %w", err)
	}

	warnings := EvaluatePalette(in.Colors)

	out, err := svc.store.UpdateSettings(ctx, in)
	if err != nil {
		return status.Settings{}, nil, err
	}

	// UpdateSettings already bumps the version inside its SQL — only clear the
	// in-memory cache here to avoid a redundant second bump.
	svc.clearCache()
	return out, warnings, nil
}

func (svc *PersonalizationService) GetAsset(ctx context.Context, role status.AssetRole) (*status.Asset, error) {
	return svc.store.GetAsset(ctx, role)
}

func (svc *PersonalizationService) PutAsset(ctx context.Context, role status.AssetRole, mime string, data []byte, altText string) error {
	a := status.Asset{
		Role:    role,
		MIME:    mime,
		Bytes:   data,
		AltText: altText,
	}
	if err := svc.store.PutAsset(ctx, a); err != nil {
		return err
	}
	svc.invalidateCache(ctx)
	return nil
}

func (svc *PersonalizationService) DeleteAsset(ctx context.Context, role status.AssetRole) error {
	if err := svc.store.DeleteAsset(ctx, role); err != nil {
		return err
	}
	svc.invalidateCache(ctx)
	return nil
}

func (svc *PersonalizationService) ListFooterLinks(ctx context.Context) ([]status.FooterLink, error) {
	return svc.store.ListFooterLinks(ctx)
}

func (svc *PersonalizationService) CreateFooterLink(ctx context.Context, label, url string) (status.FooterLink, error) {
	if err := validateFooterLink(label, url); err != nil {
		return status.FooterLink{}, err
	}
	link, err := svc.store.CreateFooterLink(ctx, label, url)
	if err != nil {
		return status.FooterLink{}, err
	}
	svc.invalidateCache(ctx)
	return link, nil
}

func (svc *PersonalizationService) UpdateFooterLink(ctx context.Context, id string, label, url string) (status.FooterLink, error) {
	if err := validateFooterLink(label, url); err != nil {
		return status.FooterLink{}, err
	}
	link, err := svc.store.UpdateFooterLink(ctx, id, label, url)
	if err != nil {
		return status.FooterLink{}, err
	}
	svc.invalidateCache(ctx)
	return link, nil
}

func (svc *PersonalizationService) DeleteFooterLink(ctx context.Context, id string) error {
	if err := svc.store.DeleteFooterLink(ctx, id); err != nil {
		return err
	}
	svc.invalidateCache(ctx)
	return nil
}

func (svc *PersonalizationService) ReorderFooterLinks(ctx context.Context, ids []string) ([]status.FooterLink, error) {
	links, err := svc.store.ReorderFooterLinks(ctx, ids)
	if err != nil {
		return nil, err
	}
	svc.invalidateCache(ctx)
	return links, nil
}

func (svc *PersonalizationService) ListFAQItems(ctx context.Context) ([]status.FAQItem, error) {
	return svc.store.ListFAQItems(ctx)
}

func (svc *PersonalizationService) CreateFAQItem(ctx context.Context, question, answerMD string) (status.FAQItem, error) {
	if err := validateFAQItem(question, answerMD); err != nil {
		return status.FAQItem{}, err
	}
	answerHTML, err := RenderFAQAnswer(answerMD)
	if err != nil {
		return status.FAQItem{}, fmt.Errorf("render answer: %w", err)
	}
	item, err := svc.store.CreateFAQItem(ctx, question, answerMD, answerHTML)
	if err != nil {
		return status.FAQItem{}, err
	}
	svc.invalidateCache(ctx)
	return item, nil
}

func (svc *PersonalizationService) UpdateFAQItem(ctx context.Context, id string, question, answerMD string) (status.FAQItem, error) {
	if err := validateFAQItem(question, answerMD); err != nil {
		return status.FAQItem{}, err
	}
	answerHTML, err := RenderFAQAnswer(answerMD)
	if err != nil {
		return status.FAQItem{}, fmt.Errorf("render answer: %w", err)
	}
	item, err := svc.store.UpdateFAQItem(ctx, id, question, answerMD, answerHTML)
	if err != nil {
		return status.FAQItem{}, err
	}
	svc.invalidateCache(ctx)
	return item, nil
}

func (svc *PersonalizationService) DeleteFAQItem(ctx context.Context, id string) error {
	if err := svc.store.DeleteFAQItem(ctx, id); err != nil {
		return err
	}
	svc.invalidateCache(ctx)
	return nil
}

func (svc *PersonalizationService) ReorderFAQItems(ctx context.Context, ids []string) ([]status.FAQItem, error) {
	items, err := svc.store.ReorderFAQItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	svc.invalidateCache(ctx)
	return items, nil
}

func (svc *PersonalizationService) clearCache() {
	svc.mu.Lock()
	svc.cache = nil
	svc.mu.Unlock()
}

func (svc *PersonalizationService) invalidateCache(ctx context.Context) {
	svc.clearCache()
	// Best-effort: bump the persisted version so the public ETag changes and
	// CDN/browser caches are invalidated. Failure here is non-fatal — the underlying
	// mutation already succeeded; clients will catch up on the next version-bumping
	// write or after the Cache-Control max-age elapses.
	_ = svc.store.BumpVersion(ctx)
}

func (svc *PersonalizationService) validateSettings(s status.Settings) error {
	if len(s.Title) < 1 || len(s.Title) > 100 {
		return fmt.Errorf("%w: title must be 1-100 chars", status.ErrFieldTooLong)
	}
	if len(s.Subtitle) > 200 {
		return fmt.Errorf("%w: subtitle max 200 chars", status.ErrFieldTooLong)
	}
	if len(s.Announcement.MessageMD) > 1000 {
		return fmt.Errorf("%w: announcement message max 1000 chars", status.ErrFieldTooLong)
	}
	if len(s.FooterTextMD) > 500 {
		return fmt.Errorf("%w: footer text max 500 chars", status.ErrFieldTooLong)
	}

	for _, hex := range []string{
		s.Colors.Background, s.Colors.Surface, s.Colors.Border, s.Colors.Text, s.Colors.Accent,
		s.Colors.StatusOperational, s.Colors.StatusDegraded, s.Colors.StatusPartialOutage, s.Colors.StatusMajorOutage,
	} {
		if !hexColorRegex.MatchString(hex) {
			return fmt.Errorf("%w: %q", status.ErrInvalidHex, hex)
		}
	}

	if s.Announcement.URL != "" && !isValidURL(s.Announcement.URL) {
		return status.ErrInvalidScheme
	}

	if s.Locale != "en" && s.Locale != "fr" {
		return status.ErrInvalidLocale
	}
	if s.DateFormat != "relative" && s.DateFormat != "absolute" {
		return status.ErrInvalidDateFormat
	}
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return status.ErrInvalidTimezone
		}
	}
	return nil
}

func validateFooterLink(label, url string) error {
	if len(label) < 1 || len(label) > 60 {
		return fmt.Errorf("%w: label must be 1-60 chars", status.ErrFieldTooLong)
	}
	if !isValidURL(url) {
		return status.ErrInvalidScheme
	}
	return nil
}

func validateFAQItem(question, answerMD string) error {
	if len(question) < 1 || len(question) > 200 {
		return fmt.Errorf("%w: question must be 1-200 chars", status.ErrFieldTooLong)
	}
	if len(answerMD) > 4000 {
		return fmt.Errorf("%w: answer max 4000 chars", status.ErrFieldTooLong)
	}
	return nil
}

func isValidURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// AssetSizeCap returns the largest upload accepted for role.
func (svc *PersonalizationService) AssetSizeCap(role status.AssetRole) int64 {
	return AssetSizeCap(role)
}

// DetectAssetMIME sniffs the upload's MIME type and refuses one role does not allow.
func (svc *PersonalizationService) DetectAssetMIME(role status.AssetRole, data []byte) (string, error) {
	return DetectAssetMIME(role, data)
}
