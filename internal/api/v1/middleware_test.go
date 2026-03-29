package v1

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// requireEnterprise middleware
// ---------------------------------------------------------------------------

func TestRequireEnterprise_CommunityBlocked(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	handler := requireEnterprise(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/status/incidents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "PRO_REQUIRED")
}

func TestRequireEnterprise_EnterpriseAllowed(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	handlerCalled := false
	handler := requireEnterprise(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/status/incidents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, handlerCalled)
}

// ---------------------------------------------------------------------------
// parseCORSOrigins
// ---------------------------------------------------------------------------

func TestParseCORSOrigins_Empty(t *testing.T) {
	assert.Nil(t, parseCORSOrigins(""))
}

func TestParseCORSOrigins_Wildcard(t *testing.T) {
	origins := parseCORSOrigins("*")
	assert.Equal(t, []string{"*"}, origins)
}

func TestParseCORSOrigins_MultipleOrigins(t *testing.T) {
	origins := parseCORSOrigins("https://a.com, https://b.com")
	assert.Equal(t, []string{"https://a.com", "https://b.com"}, origins)
}

func TestParseCORSOrigins_TrimsWhitespace(t *testing.T) {
	origins := parseCORSOrigins("  https://a.com ,, https://b.com  ")
	assert.Equal(t, []string{"https://a.com", "https://b.com"}, origins)
}

// ---------------------------------------------------------------------------
// cors middleware
// ---------------------------------------------------------------------------

func TestCORS_WildcardSetsHeader(t *testing.T) {
	handler := cors([]string{"*"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowlistMatchesOrigin(t *testing.T) {
	handler := cors([]string{"https://allowed.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://allowed.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "https://allowed.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowlistRejectsUnknownOrigin(t *testing.T) {
	handler := cors([]string{"https://allowed.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_OptionsReturns204(t *testing.T) {
	handler := cors([]string{"*"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCORS_NilOriginsNoHeaders(t *testing.T) {
	handler := cors(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://any.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// bodyLimit middleware
// ---------------------------------------------------------------------------

func TestBodyLimit_PostRequestLimited(t *testing.T) {
	handler := bodyLimit(10, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(strings.Repeat("x", 20))
	req := httptest.NewRequest("POST", "/", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestBodyLimit_GetRequestNotLimited(t *testing.T) {
	handler := bodyLimit(10, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// requestID middleware
// ---------------------------------------------------------------------------

func TestRequestID_GeneratesIDWhenMissing(t *testing.T) {
	var captured string
	handler := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, captured)
	assert.Equal(t, captured, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_PreservesExistingID(t *testing.T) {
	var captured string
	handler := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "my-custom-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "my-custom-id", captured)
	assert.Equal(t, "my-custom-id", rec.Header().Get("X-Request-ID"))
}

// ---------------------------------------------------------------------------
// panicRecovery middleware
// ---------------------------------------------------------------------------

func TestPanicRecovery_CatchesPanicReturns500(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := panicRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}), logger)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPanicRecovery_NoPanicPassesThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := panicRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), logger)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// requestLogger middleware
// ---------------------------------------------------------------------------

func TestRequestLogger_PassesThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := requestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), logger)

	req := httptest.NewRequest("POST", "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

// ---------------------------------------------------------------------------
// statusWriter preserves http.Flusher
// ---------------------------------------------------------------------------

func TestStatusWriter_ImplementsFlusher(t *testing.T) {
	// The SSE broker checks w.(http.Flusher). The requestLogger middleware wraps
	// w in a statusWriter. If statusWriter doesn't implement Flusher, SSE
	// connections get a 500 error.
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: 200}

	_, ok := interface{}(sw).(http.Flusher)
	assert.True(t, ok, "statusWriter must implement http.Flusher for SSE to work")
}

func TestMiddlewareChain_PreservesFlusherForSSE(t *testing.T) {
	// End-to-end: the full middleware chain must preserve http.Flusher so that
	// SSE handlers can stream events.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var flusherAvailable bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, flusherAvailable = w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
	})

	var h http.Handler = inner
	h = bodyLimit(1048576, h)
	h = cors([]string{"*"}, h)
	h = requestID(h)
	h = requestLogger(h, logger)
	h = panicRecovery(h, logger)

	req := httptest.NewRequest("GET", "/api/v1/containers/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, flusherAvailable, "http.Flusher must be available through the full middleware chain")
}

// ---------------------------------------------------------------------------
// Full middleware chain integration
// ---------------------------------------------------------------------------

func TestMiddlewareChain_Integration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var capturedRequestID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Build the chain as Router.Handler() does
	var h http.Handler = inner
	h = bodyLimit(1048576, h)
	h = cors([]string{"https://test.com"}, h)
	h = requestID(h)
	h = requestLogger(h, logger)
	h = panicRecovery(h, logger)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Origin", "https://test.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, capturedRequestID)
	assert.Equal(t, capturedRequestID, rec.Header().Get("X-Request-ID"))
	assert.Equal(t, "https://test.com", rec.Header().Get("Access-Control-Allow-Origin"))
}
