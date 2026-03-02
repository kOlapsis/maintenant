package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testManager creates a LicenseManager wired to a test HTTP server.
func testManager(t *testing.T, pub ed25519.PublicKey, handler http.HandlerFunc) *LicenseManager {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	m := &LicenseManager{
		licenseKey: "test-key-123",
		dataDir:    t.TempDir(),
		version:    "test",
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		publicKey:  pub,
		client:     server.Client(),
		stop:       make(chan struct{}),
	}
	m.state.Store(&LicenseState{Status: "unknown"})

	// Override the server URL for this test
	origOverride := licenseServerOverride
	licenseServerOverride = server.URL
	t.Cleanup(func() { licenseServerOverride = origOverride })

	return m
}

func TestManager_ActiveLicense(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key-123", r.Header.Get("Authorization"))
		assert.Contains(t, r.Header.Get("User-Agent"), "PulseBoard/")

		payload := LicensePayload{
			Status:     "active",
			Plan:       "pro",
			Features:   []string{"all"},
			ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
			VerifiedAt: time.Now(),
		}
		signed := signPayload(t, priv, payload)
		json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.True(t, state.IsProEnabled)
	assert.Equal(t, "active", state.Status)
	assert.Equal(t, "pro", state.Plan)
	assert.Empty(t, state.Message)
}

func TestManager_GraceLicense(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		payload := LicensePayload{
			Status:     "grace",
			Plan:       "pro",
			ExpiresAt:  time.Now().Add(-24 * time.Hour),
			VerifiedAt: time.Now(),
		}
		signed := signPayload(t, priv, payload)
		json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.True(t, state.IsProEnabled)
	assert.Equal(t, "grace", state.Status)
	assert.Contains(t, state.Message, "grace period")
}

func TestManager_ExpiredLicense(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		payload := LicensePayload{
			Status:     "expired",
			Plan:       "pro",
			ExpiresAt:  time.Now().Add(-30 * 24 * time.Hour),
			VerifiedAt: time.Now(),
		}
		signed := signPayload(t, priv, payload)
		json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.False(t, state.IsProEnabled)
	assert.Equal(t, "expired", state.Status)
	assert.Contains(t, state.Message, "expired")
}

func TestManager_UnknownKey_HTTP401(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unknown key"}`))
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.False(t, state.IsProEnabled)
	assert.Equal(t, "unknown", state.Status)
	assert.Contains(t, state.Message, "not recognized")
}

func TestManager_InvalidSignature(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	_, otherPriv := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		payload := LicensePayload{
			Status: "active",
			Plan:   "pro",
		}
		// Sign with wrong key
		signed := signPayload(t, otherPriv, payload)
		json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)

	// Set an existing state to verify it's preserved
	m.state.Store(&LicenseState{
		IsProEnabled: true,
		Status:       "active",
		Plan:         "pro",
		VerifiedAt:   time.Now(),
	})

	m.check(context.Background())

	// State should be preserved (treated as network error with recent cache)
	state := m.State()
	assert.True(t, state.IsProEnabled)
	assert.Equal(t, "active", state.Status)
}

func TestManager_NetworkError(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	m := testManager(t, pub, handler)

	// No cache, no prior state
	m.check(context.Background())
	state := m.State()
	assert.False(t, state.IsProEnabled)
	assert.Equal(t, "unreachable", state.Status)
}

func TestManager_NetworkError_CacheFallback(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	// First: serve an active license
	callCount := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			payload := LicensePayload{
				Status:     "active",
				Plan:       "pro",
				ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
				VerifiedAt: time.Now(),
			}
			signed := signPayload(t, priv, payload)
			json.NewEncoder(w).Encode(signed)
			return
		}
		// Second call: server error
		w.WriteHeader(http.StatusInternalServerError)
	}

	m := testManager(t, pub, handler)

	// First check: gets active license and caches it
	m.check(context.Background())
	assert.True(t, m.IsProEnabled())

	// Second check: server fails, should keep Pro from cache
	m.check(context.Background())
	assert.True(t, m.IsProEnabled())
}

func TestManager_CacheLoadOnStart(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	dir := t.TempDir()

	// Pre-populate cache
	payload := LicensePayload{
		Status:     "active",
		Plan:       "pro",
		Features:   []string{"all"},
		ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
		VerifiedAt: time.Now(),
	}
	signed := signPayload(t, priv, payload)
	require.NoError(t, writeCache(dir, &signed))

	// Create manager with a server that's always down
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origOverride := licenseServerOverride
	licenseServerOverride = server.URL
	defer func() { licenseServerOverride = origOverride }()

	m := &LicenseManager{
		licenseKey: "test-key-123",
		dataDir:    dir,
		version:    "test",
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		publicKey:  pub,
		client:     server.Client(),
		stop:       make(chan struct{}),
	}
	m.state.Store(&LicenseState{Status: "unknown"})

	// Start should load cache first
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	defer cancel()
	defer m.Stop()

	// Cache was loaded, Pro enabled despite server being down
	assert.True(t, m.IsProEnabled())
	assert.Equal(t, "active", m.State().Status)
}

func TestGetPublicKey_Valid(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	origKey := publicKeyB64
	publicKeyB64 = base64.StdEncoding.EncodeToString(pub)
	defer func() { publicKeyB64 = origKey }()

	key, err := getPublicKey()
	require.NoError(t, err)
	assert.Equal(t, pub, key)
}

func TestGetPublicKey_Empty(t *testing.T) {
	origKey := publicKeyB64
	publicKeyB64 = ""
	defer func() { publicKeyB64 = origKey }()

	_, err := getPublicKey()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not set")
}

func TestGetPublicKey_InvalidBase64(t *testing.T) {
	origKey := publicKeyB64
	publicKeyB64 = "not-valid-base64!!!"
	defer func() { publicKeyB64 = origKey }()

	_, err := getPublicKey()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license public key encoding")
}

func TestGetPublicKey_WrongSize(t *testing.T) {
	origKey := publicKeyB64
	publicKeyB64 = base64.StdEncoding.EncodeToString([]byte("tooshort"))
	defer func() { publicKeyB64 = origKey }()

	_, err := getPublicKey()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license public key size")
}
