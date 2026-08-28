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
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func signPayload(t *testing.T, priv ed25519.PrivateKey, payload interface{}) SignedResponse {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, data)
	return SignedResponse{
		Payload:   string(data),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
}

func TestVerify_ValidSignature(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	payload := LicensePayload{
		Status:     "active",
		Plan:       "pro",
		Features:   []string{"all"},
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour),
		VerifiedAt: time.Now(),
	}

	signed := signPayload(t, priv, payload)
	result, err := verify(pub, signed)

	require.NoError(t, err)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, "pro", result.Plan)
}

func TestVerify_WrongKey(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	otherPub, _ := generateTestKeyPair(t)

	payload := LicensePayload{
		Status: "active",
		Plan:   "pro",
	}

	signed := signPayload(t, priv, payload)
	_, err := verify(otherPub, signed)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license signature")
}

func TestVerify_TamperedPayload(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	payload := LicensePayload{
		Status: "active",
		Plan:   "pro",
	}

	signed := signPayload(t, priv, payload)
	// Tamper with the payload
	signed.Payload = `{"status":"active","plan":"enterprise","features":null,"expires_at":"0001-01-01T00:00:00Z","verified_at":"0001-01-01T00:00:00Z"}`

	_, err := verify(pub, signed)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license signature")
}

func TestVerify_InvalidSignatureEncoding(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	signed := SignedResponse{
		Payload:   `{"status":"active"}`,
		Signature: "not-valid-base64!!!",
	}

	_, err := verify(pub, signed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature encoding")
}

// TestVerify_UpdatesUntilSurvivesTheRoundTrip: the field the license server
// added for Personal must reach the payload intact and parse as a date.
func TestVerify_UpdatesUntilSurvivesTheRoundTrip(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	signed := signPayload(t, priv, map[string]any{
		"status":        "active",
		"edition":       "personal",
		"plan":          "personal",
		"updates_until": "2027-08-09T12:00:00Z",
		"verified_at":   time.Now().Format(time.RFC3339),
	})

	result, err := verify(pub, signed)
	require.NoError(t, err)
	assert.Equal(t, "2027-08-09T12:00:00Z", result.UpdatesUntil)

	end, ok := result.updateWindowEnd()
	require.True(t, ok)
	assert.Equal(t, time.Date(2027, time.August, 9, 12, 0, 0, 0, time.UTC), end.UTC())
}

// TestVerify_MalformedUpdatesUntilCostsOnlyThatField: the whole reason the field
// is a string. A date the payload cannot parse must not make verify report
// "invalid license payload", which would route a valid license into
// handleNetworkError and silently drop its edition.
func TestVerify_MalformedUpdatesUntilCostsOnlyThatField(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	for _, bad := range []string{"not-a-date", "09/08/2027", "2027-13-45T99:99:99Z", ""} {
		t.Run(bad, func(t *testing.T) {
			signed := signPayload(t, priv, map[string]any{
				"status":        "active",
				"edition":       "personal",
				"plan":          "personal",
				"updates_until": bad,
				"verified_at":   time.Now().Format(time.RFC3339),
			})

			result, err := verify(pub, signed)
			require.NoError(t, err, "a malformed window must not invalidate the payload")
			assert.Equal(t, "active", result.Status)
			assert.Equal(t, "personal", result.Edition)

			_, ok := result.updateWindowEnd()
			assert.False(t, ok, "an unreadable window must report no window")
		})
	}
}

// TestVerify_AbsentUpdatesUntilReportsNoWindow: a Pro subscription never carries
// the field, and expires_at already governs it.
func TestVerify_AbsentUpdatesUntilReportsNoWindow(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	signed := signPayload(t, priv, map[string]any{
		"status":      "active",
		"edition":     "pro",
		"plan":        "pro",
		"expires_at":  time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
		"verified_at": time.Now().Format(time.RFC3339),
	})

	result, err := verify(pub, signed)
	require.NoError(t, err)
	assert.Empty(t, result.UpdatesUntil)

	_, ok := result.updateWindowEnd()
	assert.False(t, ok)
}

// TestResolveEdition covers the five resolution rules of
// contracts/license-payload.md, in order.
func TestResolveEdition(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name    string
		payload *LicensePayload
		want    extension.Edition
	}{
		// Rule 1 — declared and recognised.
		{"declared community", &LicensePayload{Status: "active", Edition: "community"}, extension.Community},
		{"declared personal", &LicensePayload{Status: "active", Edition: "personal"}, extension.Personal},
		{"declared pro", &LicensePayload{Status: "active", Edition: "pro"}, extension.Pro},

		// Rule 2 — declared but unknown. The most restrictive answer wins, and it
		// must not be confused with rule 4, which requires the field to be absent.
		{"declared unknown", &LicensePayload{Status: "active", Edition: "enterprise"}, extension.Community},
		{"declared unknown on a pro plan", &LicensePayload{Status: "active", Edition: "enterprise", Plan: "pro"}, extension.Community},

		// Rule 3 — no edition, plan carries it.
		{"plan personal", &LicensePayload{Status: "active", Plan: "personal"}, extension.Personal},

		// Rule 4 — the compatibility clause: every license in service today.
		{"no edition, active", &LicensePayload{Status: "active", Plan: "pro"}, extension.Pro},
		{"no edition, grace", &LicensePayload{Status: "grace", Plan: "pro"}, extension.Pro},
		{"no edition, no plan, active", &LicensePayload{Status: "active"}, extension.Pro},

		// Rule 5 — nothing usable.
		{"expired", &LicensePayload{Status: "expired", Plan: "pro"}, extension.Community},
		{"revoked", &LicensePayload{Status: "revoked", Plan: "pro"}, extension.Community},
		{"nil payload", nil, extension.Community},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveEdition(c.payload, discard); got != c.want {
				t.Errorf("ResolveEdition = %q, want %q", got, c.want)
			}
		})
	}
}

// TestResolveEdition_UnknownEditionIsLogged: FR-010 asks for the discrepancy to
// be recorded, not swallowed.
func TestResolveEdition_UnknownEditionIsLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ResolveEdition(&LicensePayload{Status: "active", Edition: "enterprise"}, logger)

	if !strings.Contains(buf.String(), "enterprise") {
		t.Errorf("the unknown edition was not logged; got %q", buf.String())
	}
}

// TestResolveEdition_NoExpiryIsNotAnExpiry: a perpetual license carries no end
// date, and a zero date must never be read as "expired" (FR-008).
func TestResolveEdition_NoExpiryIsNotAnExpiry(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	perpetual := &LicensePayload{
		Status:     "active",
		Edition:    "personal",
		VerifiedAt: time.Now(),
		// ExpiresAt deliberately left at its zero value.
	}
	if !perpetual.ExpiresAt.IsZero() {
		t.Fatal("precondition: the payload must carry no expiry")
	}
	if got := ResolveEdition(perpetual, discard); got != extension.Personal {
		t.Errorf("a perpetual Personal license resolved to %q, want %q", got, extension.Personal)
	}
}
