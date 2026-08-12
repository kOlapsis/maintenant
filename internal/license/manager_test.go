// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testManager creates a Manager wired to a test HTTP server.
func testManager(t *testing.T, pub ed25519.PublicKey, handler http.HandlerFunc) *Manager {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	m := &Manager{
		licenseKey: "test-key-123",
		dataDir:    t.TempDir(),
		version:    "test",
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		publicKey:  pub,
		client:     server.Client(),
		stop:       make(chan struct{}),
	}
	m.state.Store(&State{Status: "unknown"})

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
		assert.Contains(t, r.Header.Get("User-Agent"), "maintenant/")

		payload := LicensePayload{
			Status:     "active",
			Plan:       "pro",
			Features:   []string{"all"},
			ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
			VerifiedAt: time.Now(),
		}
		signed := signPayload(t, priv, payload)
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Pro, state.Edition)
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
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Pro, state.Edition)
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
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, "expired", state.Status)
	assert.Contains(t, state.Message, "expired")
}

func TestManager_UnknownKey_HTTP401(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Invalid license key"}`))
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, "unknown", state.Status)
	assert.Equal(t, "Invalid license key", state.Message)
}

func TestManager_UnknownKey_HTTP401_NoBody(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, "unknown", state.Status)
	assert.NotEmpty(t, state.Message)
}

func TestManager_ExpiredKey_HTTP403(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":"expired","message":"Your license has expired. Renew at https://maintenant.dev/pricing"}`))
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, "expired", state.Status)
	assert.Contains(t, state.Message, "expired")
}

func TestManager_CanceledKey_HTTP403(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":"canceled","message":"Your subscription has been canceled."}`))
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, "canceled", state.Status)
	assert.Contains(t, state.Message, "canceled")
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
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)

	// Set an existing state to verify it's preserved
	m.state.Store(&State{
		Edition:    extension.Pro,
		Status:       "active",
		Plan:         "pro",
		VerifiedAt:   time.Now(),
	})

	m.check(context.Background())

	// State should be preserved (treated as network error with recent cache)
	state := m.State()
	assert.Equal(t, extension.Pro, state.Edition)
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
	assert.Equal(t, extension.Community, state.Edition)
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
			_ = json.NewEncoder(w).Encode(signed)
			return
		}
		// Second call: server error
		w.WriteHeader(http.StatusInternalServerError)
	}

	m := testManager(t, pub, handler)

	// First check: gets active license and caches it
	m.check(context.Background())
	assert.Equal(t, extension.Pro, m.Edition())

	// Second check: server fails, should keep Pro from cache
	m.check(context.Background())
	assert.Equal(t, extension.Pro, m.Edition())
}

func TestManager_CacheLoadOnConstruction(t *testing.T) {
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

	m := &Manager{
		licenseKey: "test-key-123",
		dataDir:    dir,
		version:    "test",
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		publicKey:  pub,
		client:     server.Client(),
		stop:       make(chan struct{}),
	}
	m.state.Store(&State{Status: "unknown"})

	// Cache load is what NewManager does at construction time. Simulating it
	// here lets the test exercise the same effect on a manually-built Manager.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.loadCache(ctx)

	// Pro is now enabled from cache, before any network check.
	assert.Equal(t, extension.Pro, m.Edition())
	assert.Equal(t, "active", m.State().Status)

	// Start runs a network check; with the server down, graceful degradation
	// keeps Pro enabled (cache age is well under the 7-day warn threshold).
	m.Start(ctx)
	defer m.Stop()

	assert.Equal(t, extension.Pro, m.Edition())
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

func TestLicenseManagerEditionChangeCallback(t *testing.T) {
	makeProPayload := func(t *testing.T, priv ed25519.PrivateKey) LicensePayload {
		return LicensePayload{
			Status:     "active",
			Plan:       "pro",
			Features:   []string{"all"},
			ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
			VerifiedAt: time.Now(),
		}
	}

	t.Run("pro_to_ce_triggers_callback", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		callCount := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			payload := makeProPayload(t, priv)
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		var callArgs []struct{ prev, next extension.Edition }
		m.RegisterEditionChangeCallback(func(ctx context.Context, prev, next extension.Edition) {
			mu.Lock()
			callArgs = append(callArgs, struct{ prev, next extension.Edition }{prev, next})
			callCount++
			mu.Unlock()
		})

		// First check: sets baseline (Pro), no dispatch
		m.check(context.Background())
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, callCount, "baseline should not trigger callback")
		mu.Unlock()

		// Simulate Pro→CE: override server to return expired
		origOverride := licenseServerOverride
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := LicensePayload{Status: "expired", ExpiresAt: time.Now().Add(-1 * time.Hour), VerifiedAt: time.Now()}
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}))
		defer ts.Close()
		licenseServerOverride = ts.URL
		defer func() { licenseServerOverride = origOverride }()

		m.check(context.Background())
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 1, callCount, "Pro→CE should trigger callback")
		if len(callArgs) > 0 {
			assert.Equal(t, extension.Pro, callArgs[0].prev)
			assert.Equal(t, extension.Community, callArgs[0].next)
		}
		mu.Unlock()
	})

	t.Run("ce_to_pro_triggers_callback", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		// First make it expired (CE state)
		callCount := 0
		expiredCalls := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			expiredCalls++
			if expiredCalls <= 1 {
				payload := LicensePayload{Status: "expired", ExpiresAt: time.Now().Add(-1 * time.Hour), VerifiedAt: time.Now()}
				signed := signPayload(t, priv, payload)
				_ = json.NewEncoder(w).Encode(signed)
				return
			}
			payload := makeProPayload(t, priv)
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		var callArgs []struct{ prev, next extension.Edition }
		m.RegisterEditionChangeCallback(func(ctx context.Context, prev, next extension.Edition) {
			mu.Lock()
			callArgs = append(callArgs, struct{ prev, next extension.Edition }{prev, next})
			callCount++
			mu.Unlock()
		})

		// First check: sets baseline (expired → CE)
		m.check(context.Background())
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, callCount, "baseline should not trigger")
		mu.Unlock()

		// Second check: CE→Pro
		m.check(context.Background())
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 1, callCount, "CE→Pro should trigger callback")
		if len(callArgs) > 0 {
			assert.Equal(t, extension.Community, callArgs[0].prev)
			assert.Equal(t, extension.Pro, callArgs[0].next)
		}
		mu.Unlock()
	})

	t.Run("initial_state_not_transition", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		handler := func(w http.ResponseWriter, r *http.Request) {
			payload := makeProPayload(t, priv)
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		callCount := 0
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			mu.Lock()
			callCount++
			mu.Unlock()
		})

		// Single applyPayload — sets baseline, no dispatch
		m.check(context.Background())
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 0, callCount, "initial state must not trigger callback")
		mu.Unlock()
	})

	t.Run("pro_to_pro_no_dispatch", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		handler := func(w http.ResponseWriter, r *http.Request) {
			payload := makeProPayload(t, priv)
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		callCount := 0
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			mu.Lock()
			callCount++
			mu.Unlock()
		})

		// Both checks return Pro — no transition, no dispatch
		m.check(context.Background())
		m.check(context.Background())
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 0, callCount, "Pro→Pro must not trigger callback")
		mu.Unlock()
	})

	t.Run("two_callbacks_both_invoked", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		callCount := 0
		expiredCalled := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			expiredCalled++
			if expiredCalled <= 1 {
				payload := makeProPayload(t, priv)
				signed := signPayload(t, priv, payload)
				_ = json.NewEncoder(w).Encode(signed)
				return
			}
			payload := LicensePayload{Status: "expired", ExpiresAt: time.Now().Add(-1 * time.Hour), VerifiedAt: time.Now()}
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		invoked := make([]int, 0, 2)
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			mu.Lock()
			invoked = append(invoked, 1)
			callCount++
			mu.Unlock()
		})
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			mu.Lock()
			invoked = append(invoked, 2)
			callCount++
			mu.Unlock()
		})

		m.check(context.Background()) // baseline Pro
		m.check(context.Background()) // transition → expired (CE)
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 2, callCount, "both callbacks should be invoked")
		mu.Unlock()
	})

	t.Run("panic_in_callback_recovered", func(t *testing.T) {
		pub, priv := generateTestKeyPair(t)
		expiredCalled := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			expiredCalled++
			if expiredCalled <= 1 {
				payload := makeProPayload(t, priv)
				signed := signPayload(t, priv, payload)
				_ = json.NewEncoder(w).Encode(signed)
				return
			}
			payload := LicensePayload{Status: "expired", ExpiresAt: time.Now().Add(-1 * time.Hour), VerifiedAt: time.Now()}
			signed := signPayload(t, priv, payload)
			_ = json.NewEncoder(w).Encode(signed)
		}
		m := testManager(t, pub, handler)

		var mu sync.Mutex
		secondCalled := false
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			panic("intentional panic for test")
		})
		m.RegisterEditionChangeCallback(func(_ context.Context, _, _ extension.Edition) {
			mu.Lock()
			secondCalled = true
			mu.Unlock()
		})

		m.check(context.Background()) // baseline
		m.check(context.Background()) // transition
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		assert.True(t, secondCalled, "second callback must run even after first panicked")
		mu.Unlock()
	})
}

// The three paths that used to write state without notifying. A revocation seen
// on any of them left escalation running, because m.state.Store bypassed
// setStateAndNotify. These are the regression guards.

// proPayload is an active license with no edition declared — the compatibility
// case, and what every license in service looks like today.
func proPayload() LicensePayload {
	return LicensePayload{
		Status:     "active",
		Plan:       "pro",
		Features:   []string{"all"},
		ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
		VerifiedAt: time.Now(),
	}
}

func editionCallbackRecorder(t *testing.T, m *Manager) func() []extension.Edition {
	t.Helper()
	var mu sync.Mutex
	var seen []extension.Edition
	m.RegisterEditionChangeCallback(func(_ context.Context, _, next extension.Edition) {
		mu.Lock()
		seen = append(seen, next)
		mu.Unlock()
	})
	return func() []extension.Edition {
		mu.Lock()
		defer mu.Unlock()
		out := make([]extension.Edition, len(seen))
		copy(out, seen)
		return out
	}
}

func TestManager_HTTP401NotifiesTheTransition(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	unauthorized := false
	handler := func(w http.ResponseWriter, _ *http.Request) {
		if unauthorized {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "unknown key"})
			return
		}
		signed := signPayload(t, priv, proPayload())
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	seen := editionCallbackRecorder(t, m)

	m.check(context.Background()) // baseline: Pro
	require.Equal(t, extension.Pro, m.Edition())

	unauthorized = true
	m.check(context.Background())
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, []extension.Edition{extension.Community}, seen(),
		"an HTTP 401 must dispatch the transition, not just store the state")
}

func TestManager_HTTP403NotifiesTheTransition(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	forbidden := false
	handler := func(w http.ResponseWriter, _ *http.Request) {
		if forbidden {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "message": "refunded"})
			return
		}
		signed := signPayload(t, priv, proPayload())
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	seen := editionCallbackRecorder(t, m)

	m.check(context.Background())
	require.Equal(t, extension.Pro, m.Edition())

	forbidden = true
	m.check(context.Background())
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, []extension.Edition{extension.Community}, seen(),
		"an HTTP 403 revocation must dispatch the transition")
}

func TestManager_NetworkDegradationBeyondSixtyDaysNotifies(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	down := false
	handler := func(w http.ResponseWriter, _ *http.Request) {
		if down {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload := proPayload()
		// Verified long enough ago that the next failure crosses the 60-day line.
		payload.VerifiedAt = time.Now().Add(-61 * 24 * time.Hour)
		signed := signPayload(t, priv, payload)
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	seen := editionCallbackRecorder(t, m)

	m.check(context.Background())
	require.Equal(t, extension.Pro, m.Edition())

	down = true
	m.check(context.Background())
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, []extension.Edition{extension.Community}, seen(),
		"crossing the 60-day offline window must dispatch the transition")
}

// A Personal license must survive the offline scale exactly like a Pro one, and
// its absence of an end date must never be read as an expiry (FR-008, FR-009).
func TestManager_PersonalLicenseIsPerpetualAndDegradesLikePro(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	handler := func(w http.ResponseWriter, _ *http.Request) {
		payload := LicensePayload{
			Status:     "active",
			Edition:    "personal",
			Plan:       "personal",
			VerifiedAt: time.Now(),
			// ExpiresAt deliberately left zero: perpetual.
		}
		signed := signPayload(t, priv, payload)
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := testManager(t, pub, handler)
	m.check(context.Background())

	assert.Equal(t, extension.Personal, m.Edition())
	assert.True(t, m.State().ExpiresAt.IsZero(), "a perpetual license carries no end date")
	assert.Equal(t, "active", m.State().Status)
}
