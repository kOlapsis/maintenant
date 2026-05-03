package status

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPublicHandler(t *testing.T) *PersonalizationPublicHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewPersonalizationService(newMockPersonalizationStore(), logger)
	return NewPersonalizationPublicHandler(svc, logger)
}

func withEnterprise(t *testing.T) func() {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	return func() { extension.CurrentEdition = original }
}

func TestPersonalizationPublicHandler_CacheControlHeaders(t *testing.T) {
	defer withEnterprise(t)()
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
	defer withEnterprise(t)()
	h := newTestPublicHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec := httptest.NewRecorder()
	h.HandleSettingsJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("ETag"))
}

func TestPersonalizationPublicHandler_304OnMatchingETag(t *testing.T) {
	defer withEnterprise(t)()
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
	defer withEnterprise(t)()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewPersonalizationService(newMockPersonalizationStore(), logger)
	h := NewPersonalizationPublicHandler(svc, logger)

	req1 := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec1 := httptest.NewRecorder()
	h.HandleSettingsJSON(rec1, req1)
	etag1 := rec1.Header().Get("ETag")

	in := DefaultSettings()
	in.Title = "Updated"
	_, _, err := svc.UpdateSettings(req1.Context(), in)
	require.NoError(t, err)

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
	store := newMockPersonalizationStore()
	// Simulate enterprise customization already in DB
	store.settings.Title = "Enterprise Custom Title"
	svc := NewPersonalizationService(store, logger)
	h := NewPersonalizationPublicHandler(svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/status/settings.json", nil)
	rec := httptest.NewRecorder()
	h.HandleSettingsJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// CE must return defaults, not the stored enterprise title
	assert.NotContains(t, rec.Body.String(), "Enterprise Custom Title")
	assert.Contains(t, rec.Body.String(), "System Status")
}
