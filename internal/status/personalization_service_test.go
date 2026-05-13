package status

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- In-memory store for service tests ---

type mockPersonalizationStore struct {
	settings    Settings
	assets      map[AssetRole]*Asset
	footerLinks []FooterLink
	faqItems    []FAQItem
	nextID      int64
}

func newMockPersonalizationStore() *mockPersonalizationStore {
	return &mockPersonalizationStore{
		settings: DefaultSettings(),
		assets:   make(map[AssetRole]*Asset),
	}
}

func (m *mockPersonalizationStore) GetSettings(_ context.Context) (Settings, error) {
	return m.settings, nil
}

func (m *mockPersonalizationStore) UpdateSettings(_ context.Context, s Settings) (Settings, error) {
	s.Version = m.settings.Version + 1
	s.UpdatedAt = time.Now().UTC()
	m.settings = s
	return m.settings, nil
}

func (m *mockPersonalizationStore) BumpVersion(_ context.Context) error {
	m.settings.Version++
	m.settings.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockPersonalizationStore) GetAsset(_ context.Context, role AssetRole) (*Asset, error) {
	return m.assets[role], nil
}

func (m *mockPersonalizationStore) PutAsset(_ context.Context, a Asset) error {
	m.assets[a.Role] = &a
	return nil
}

func (m *mockPersonalizationStore) DeleteAsset(_ context.Context, role AssetRole) error {
	delete(m.assets, role)
	return nil
}

func (m *mockPersonalizationStore) ListFooterLinks(_ context.Context) ([]FooterLink, error) {
	return m.footerLinks, nil
}

func (m *mockPersonalizationStore) CreateFooterLink(_ context.Context, label, url string) (FooterLink, error) {
	m.nextID++
	l := FooterLink{ID: m.nextID, Label: label, URL: url, Position: len(m.footerLinks) + 1}
	m.footerLinks = append(m.footerLinks, l)
	return l, nil
}

func (m *mockPersonalizationStore) UpdateFooterLink(_ context.Context, id int64, label, url string) (FooterLink, error) {
	for i, l := range m.footerLinks {
		if l.ID == id {
			m.footerLinks[i].Label = label
			m.footerLinks[i].URL = url
			return m.footerLinks[i], nil
		}
	}
	return FooterLink{}, ErrNotFound
}

func (m *mockPersonalizationStore) DeleteFooterLink(_ context.Context, id int64) error {
	for i, l := range m.footerLinks {
		if l.ID == id {
			m.footerLinks = append(m.footerLinks[:i], m.footerLinks[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockPersonalizationStore) ReorderFooterLinks(_ context.Context, _ []int64) ([]FooterLink, error) {
	return m.footerLinks, nil
}

func (m *mockPersonalizationStore) ListFAQItems(_ context.Context) ([]FAQItem, error) {
	return m.faqItems, nil
}

func (m *mockPersonalizationStore) CreateFAQItem(_ context.Context, question, answerMD, answerHTML string) (FAQItem, error) {
	m.nextID++
	item := FAQItem{ID: m.nextID, Question: question, AnswerMD: answerMD, AnswerHTML: answerHTML, Position: len(m.faqItems) + 1}
	m.faqItems = append(m.faqItems, item)
	return item, nil
}

func (m *mockPersonalizationStore) UpdateFAQItem(_ context.Context, id int64, question, answerMD, answerHTML string) (FAQItem, error) {
	for i, item := range m.faqItems {
		if item.ID == id {
			m.faqItems[i].Question = question
			m.faqItems[i].AnswerMD = answerMD
			m.faqItems[i].AnswerHTML = answerHTML
			return m.faqItems[i], nil
		}
	}
	return FAQItem{}, ErrNotFound
}

func (m *mockPersonalizationStore) DeleteFAQItem(_ context.Context, id int64) error {
	for i, item := range m.faqItems {
		if item.ID == id {
			m.faqItems = append(m.faqItems[:i], m.faqItems[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockPersonalizationStore) ReorderFAQItems(_ context.Context, _ []int64) ([]FAQItem, error) {
	return m.faqItems, nil
}

func newTestPersonalizationService(t *testing.T) *PersonalizationService {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewPersonalizationService(newMockPersonalizationStore(), logger)
}

// --- Service tests ---

func TestPersonalizationService_GetSettings_Defaults(t *testing.T) {
	svc := newTestPersonalizationService(t)
	s, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "System Status", s.Title)
	assert.Equal(t, "en", s.Locale)
	assert.Equal(t, "relative", s.DateFormat)
}

func TestPersonalizationService_UpdateSettings_VersionBumps(t *testing.T) {
	svc := newTestPersonalizationService(t)
	original, err := svc.GetSettings(context.Background())
	require.NoError(t, err)

	in := DefaultSettings()
	in.Title = "Updated Title"
	out, _, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", out.Title)
	assert.Greater(t, out.Version, original.Version)
}

func TestPersonalizationService_UpdateSettings_CacheInvalidated(t *testing.T) {
	svc := newTestPersonalizationService(t)

	_, _ = svc.GetSettings(context.Background()) // prime cache

	in := DefaultSettings()
	in.Title = "After Update"
	out, _, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)

	s, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, out.Version, s.Version)
}

func TestPersonalizationService_UpdateSettings_RendersAnnouncementHTML(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Announcement.MessageMD = "**Hello** world"
	out, _, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
	assert.Contains(t, out.Announcement.MessageHTML, "<strong>")
}

func TestPersonalizationService_UpdateSettings_InvalidHex(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Colors.Background = "not-a-hex"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidHex)
}

func TestPersonalizationService_UpdateSettings_InvalidLocale(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Locale = "de"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidLocale)
}

func TestPersonalizationService_UpdateSettings_ValidLocales(t *testing.T) {
	svc := newTestPersonalizationService(t)
	for _, locale := range []string{"en", "fr"} {
		in := DefaultSettings()
		in.Locale = locale
		_, _, err := svc.UpdateSettings(context.Background(), in)
		assert.NoError(t, err, "locale %q should be valid", locale)
	}
}

func TestPersonalizationService_UpdateSettings_InvalidDateFormat(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.DateFormat = "unknown"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidDateFormat)
}

func TestPersonalizationService_UpdateSettings_ValidDateFormats(t *testing.T) {
	svc := newTestPersonalizationService(t)
	for _, fmt := range []string{"relative", "absolute"} {
		in := DefaultSettings()
		in.DateFormat = fmt
		_, _, err := svc.UpdateSettings(context.Background(), in)
		assert.NoError(t, err, "date_format %q should be valid", fmt)
	}
}

func TestPersonalizationService_UpdateSettings_InvalidTimezone(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Timezone = "Not/A/Real/Timezone"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidTimezone)
}

func TestPersonalizationService_UpdateSettings_ValidTimezone(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Timezone = "Europe/Paris"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
}

func TestPersonalizationService_UpdateSettings_EmptyTimezoneOk(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Timezone = ""
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
}

func TestPersonalizationService_UpdateSettings_InvalidAnnouncementURL(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	in.Announcement.URL = "javascript:alert(1)"
	_, _, err := svc.UpdateSettings(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidScheme)
}

func TestPersonalizationService_UpdateSettings_ContrastWarnings(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings()
	// black text on black background — contrast ratio 1:1
	in.Colors.Background = "#000000"
	in.Colors.Text = "#000000"
	_, warnings, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
	assert.NotEmpty(t, warnings, "expect contrast warnings for black-on-black palette")
}

func TestPersonalizationService_UpdateSettings_GoodPaletteNoWarnings(t *testing.T) {
	svc := newTestPersonalizationService(t)
	in := DefaultSettings() // default palette passes WCAG AA
	_, warnings, err := svc.UpdateSettings(context.Background(), in)
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestPersonalizationService_FooterLink_CRUD(t *testing.T) {
	svc := newTestPersonalizationService(t)
	ctx := context.Background()

	link, err := svc.CreateFooterLink(ctx, "Blog", "https://blog.example.com")
	require.NoError(t, err)
	assert.Equal(t, "Blog", link.Label)

	links, err := svc.ListFooterLinks(ctx)
	require.NoError(t, err)
	require.Len(t, links, 1)

	updated, err := svc.UpdateFooterLink(ctx, link.ID, "Blog Updated", "https://blog.example.com")
	require.NoError(t, err)
	assert.Equal(t, "Blog Updated", updated.Label)

	err = svc.DeleteFooterLink(ctx, link.ID)
	require.NoError(t, err)

	links, err = svc.ListFooterLinks(ctx)
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestPersonalizationService_FooterLink_InvalidURL(t *testing.T) {
	svc := newTestPersonalizationService(t)
	_, err := svc.CreateFooterLink(context.Background(), "Bad", "ftp://bad.com")
	require.ErrorIs(t, err, ErrInvalidScheme)
}

func TestPersonalizationService_FAQ_CRUD(t *testing.T) {
	svc := newTestPersonalizationService(t)
	ctx := context.Background()

	item, err := svc.CreateFAQItem(ctx, "What is this?", "A monitoring tool.")
	require.NoError(t, err)
	assert.Equal(t, "What is this?", item.Question)
	assert.NotEmpty(t, item.AnswerHTML)

	updated, err := svc.UpdateFAQItem(ctx, item.ID, "What is this?", "**Updated** answer.")
	require.NoError(t, err)
	assert.Contains(t, updated.AnswerHTML, "<strong>")

	err = svc.DeleteFAQItem(ctx, item.ID)
	require.NoError(t, err)

	items, err := svc.ListFAQItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, items)
}
