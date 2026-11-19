// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSignedRefusal(t *testing.T, w http.ResponseWriter, priv ed25519.PrivateKey, code int, payload LicensePayload) {
	t.Helper()
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(signPayload(t, priv, payload))
}

func proManagerWithCache(t *testing.T, handler http.HandlerFunc, pub ed25519.PublicKey, priv ed25519.PrivateKey) *Manager {
	t.Helper()
	m := testManager(t, pub, handler)
	signed := signPayload(t, priv, proPayload())
	require.NoError(t, writeCache(m.dataDir, &signed))
	m.loadCache(context.Background())
	require.Equal(t, extension.Pro, m.Edition())
	return m
}

func assertRefusalIgnored(t *testing.T, m *Manager) {
	t.Helper()
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Pro, state.Edition)
	assert.Equal(t, "active", state.Status)
	assert.Empty(t, state.Message, "a recent cache degrades with no message")
	_, err := os.Stat(cachePath(m.dataDir))
	assert.NoError(t, err, "the cache must survive an unverified refusal")
}

func TestManager_UnsignedRefusalKeepsProAndCache(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
	}{
		{"403 plain JSON", http.StatusForbidden, `{"status":"revoked","message":"Your license has been revoked."}`},
		{"403 empty body", http.StatusForbidden, ``},
		{"403 HTML from a proxy", http.StatusForbidden, `<html>Forbidden</html>`},
		{"401 plain JSON", http.StatusUnauthorized, `{"message":"Invalid license key"}`},
		{"401 empty body", http.StatusUnauthorized, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}
			assertRefusalIgnored(t, proManagerWithCache(t, handler, pub, priv))
		})
	}
}

func TestManager_RefusalSignedWithWrongKeyIsTreatedAsUnsigned(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusUnauthorized} {
		pub, priv := generateTestKeyPair(t)
		_, otherPriv := generateTestKeyPair(t)
		handler := func(w http.ResponseWriter, _ *http.Request) {
			writeSignedRefusal(t, w, otherPriv, code, LicensePayload{Status: "revoked", Message: "forged"})
		}
		assertRefusalIgnored(t, proManagerWithCache(t, handler, pub, priv))
	}
}

func TestManager_RefusalCarryingAGrantingPayloadIsIgnored(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	handler := func(w http.ResponseWriter, _ *http.Request) {
		writeSignedRefusal(t, w, priv, http.StatusForbidden, proPayload())
	}
	assertRefusalIgnored(t, proManagerWithCache(t, handler, pub, priv))
}

func TestManager_UnsignedRefusalDegradesLikeANetworkError(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	m := testManager(t, pub, handler)
	payload := proPayload()
	payload.VerifiedAt = time.Now().Add(-8 * 24 * time.Hour)
	signed := signPayload(t, priv, payload)
	require.NoError(t, writeCache(m.dataDir, &signed))
	m.loadCache(context.Background())

	m.check(context.Background())

	assert.Equal(t, extension.Pro, m.Edition())
	assert.Contains(t, m.State().Message, "remains active from cache")
}

func TestManager_UnsignedRefusalWithoutCacheIsUnreachable(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Invalid license key"}`))
	}
	m := testManager(t, pub, handler)
	m.check(context.Background())

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, "unreachable", m.State().Status)
}

func TestManager_SignedRefusalDowngradesAndDeletesCache(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		payload    LicensePayload
		wantStatus string
	}{
		{"403 revoked", http.StatusForbidden, LicensePayload{Status: "revoked", Message: "Your license has been revoked."}, "revoked"},
		{"403 canceled", http.StatusForbidden, LicensePayload{Status: "canceled", Message: "Your subscription has been canceled."}, "canceled"},
		{"401 unknown key", http.StatusUnauthorized, LicensePayload{Message: "Invalid license key"}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			handler := func(w http.ResponseWriter, _ *http.Request) {
				writeSignedRefusal(t, w, priv, tc.code, tc.payload)
			}
			m := proManagerWithCache(t, handler, pub, priv)

			m.check(context.Background())

			state := m.State()
			assert.Equal(t, extension.Community, state.Edition)
			assert.Equal(t, tc.wantStatus, state.Status)
			assert.Equal(t, tc.payload.Message, state.Message)
			_, err := os.Stat(cachePath(m.dataDir))
			assert.True(t, os.IsNotExist(err), "the cache must be deleted")
		})
	}
}
