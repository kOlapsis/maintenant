package status

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPersonalizationReader struct {
	settings Settings
}

func (r *stubPersonalizationReader) GetSettings(context.Context) (Settings, error) {
	return r.settings, nil
}

func (r *stubPersonalizationReader) GetAsset(context.Context, AssetRole) (*Asset, error) {
	return nil, nil
}

func (r *stubPersonalizationReader) ListFooterLinks(context.Context) ([]FooterLink, error) {
	return nil, nil
}

func (r *stubPersonalizationReader) ListFAQItems(context.Context) ([]FAQItem, error) {
	return nil, nil
}

func newTestPublicHandler(t *testing.T) *PersonalizationPublicHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewPersonalizationPublicHandler(&stubPersonalizationReader{settings: DefaultSettings()}, logger)
}

func withPro(t *testing.T) func() {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	return func() { extension.CurrentEdition = original }
}

func TestPersonalizationPublicHandler_CacheControlHeaders(t *testing.T) {
	defer withPro(t)()
	h := newTestPublicHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec := httptest.NewRecorder()
	h.HandleSettingsJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	cc := rec.Header().Get("Cache-Control")
	assert.Contains(t, cc, "public")
	assert.Contains(t, cc, "max-age=30")
	assert.Contains(t, cc, "stale-while-revalidate=60")
}

func TestPersonalizationPublicHandler_ETagPresent(t *testing.T) {
	defer withPro(t)()
	h := newTestPublicHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec := httptest.NewRecorder()
	h.HandleSettingsJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("ETag"))
}

func TestPersonalizationPublicHandler_304OnMatchingETag(t *testing.T) {
	defer withPro(t)()
	h := newTestPublicHandler(t)

	// First request — capture ETag
	req1 := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec1 := httptest.NewRecorder()
	h.HandleSettingsJSON(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	etag := rec1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Second request — same ETag via If-None-Match
	req2 := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.HandleSettingsJSON(rec2, req2)

	assert.Equal(t, http.StatusNotModified, rec2.Code)
}

func TestPersonalizationPublicHandler_ETagChangesAfterUpdate(t *testing.T) {
	defer withPro(t)()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reader := &stubPersonalizationReader{settings: DefaultSettings()}
	h := NewPersonalizationPublicHandler(reader, logger)

	req1 := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec1 := httptest.NewRecorder()
	h.HandleSettingsJSON(rec1, req1)
	etag1 := rec1.Header().Get("ETag")

	reader.settings.Title = "Updated"
	reader.settings.Version++

	req2 := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec2 := httptest.NewRecorder()
	h.HandleSettingsJSON(rec2, req2)
	etag2 := rec2.Header().Get("ETag")

	assert.NotEqual(t, etag1, etag2, "ETag must change after settings update")
}

func TestPersonalizationPublicHandler_DefaultsUnderCommunityEdition(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reader := &stubPersonalizationReader{settings: DefaultSettings()}
	reader.settings.Title = "Pro Custom Title"
	h := NewPersonalizationPublicHandler(reader, logger)

	req := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec := httptest.NewRecorder()
	h.HandleSettingsJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// CE must return defaults, not the stored pro title
	assert.NotContains(t, rec.Body.String(), "Pro Custom Title")
	assert.Contains(t, rec.Body.String(), "System Status")
}
