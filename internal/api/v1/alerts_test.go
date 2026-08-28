// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type stubChannelStore struct {
	ch *alert.NotificationChannel
}

func (s *stubChannelStore) InsertChannel(_ context.Context, ch *alert.NotificationChannel) (string, error) {
	return "1", nil
}
func (s *stubChannelStore) GetChannel(_ context.Context, _ string) (*alert.NotificationChannel, error) {
	return s.ch, nil
}
func (s *stubChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (s *stubChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (s *stubChannelStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (s *stubChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (s *stubChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "1", nil
}
func (s *stubChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (s *stubChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// HandleCreateChannel — Pro channel type guard
// ---------------------------------------------------------------------------

func TestHandleCreateChannel_TypeGating(t *testing.T) {
	tests := []struct {
		name         string
		edition      extension.Edition
		body         string
		wantStatus   int
		wantCode     string
		wantFeature  string
		wantRequired string
	}{
		{
			name:         "community + slack blocked",
			edition:      extension.Community,
			body:         `{"type":"slack","name":"test","url":"https://example.com"}`,
			wantStatus:   http.StatusForbidden,
			wantCode:     "EDITION_REQUIRED",
			wantFeature:  "slack",
			wantRequired: "pro",
		},
		{
			name:         "community + teams blocked",
			edition:      extension.Community,
			body:         `{"type":"teams","name":"test","url":"https://example.com"}`,
			wantStatus:   http.StatusForbidden,
			wantCode:     "EDITION_REQUIRED",
			wantFeature:  "teams",
			wantRequired: "pro",
		},
		{
			// Email is Personal, not Pro. The refusal must say so rather than
			// pushing every blocked channel towards the top tier.
			name:         "community + email blocked, names Personal",
			edition:      extension.Community,
			body:         `{"type":"email","name":"test","url":"https://example.com"}`,
			wantStatus:   http.StatusForbidden,
			wantCode:     "EDITION_REQUIRED",
			wantFeature:  "smtp",
			wantRequired: "personal",
		},
		{
			// And a Personal instance gets the email channel it paid for.
			name:       "personal + email passes type check",
			edition:    extension.Personal,
			body:       `{"type":"email","name":"test","url":"not-an-address"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:         "personal + slack still blocked",
			edition:      extension.Personal,
			body:         `{"type":"slack","name":"test","url":"https://example.com"}`,
			wantStatus:   http.StatusForbidden,
			wantCode:     "EDITION_REQUIRED",
			wantFeature:  "slack",
			wantRequired: "pro",
		},
		{
			name:    "community + webhook passes type check",
			edition: extension.Community,
			// http:// URL fails HTTPS scheme check → 400, not 403
			body:       `{"type":"webhook","name":"test","url":"http://not-https.example"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:    "pro + slack passes type check",
			edition: extension.Pro,
			// http:// URL fails HTTPS scheme check → 400, not 403
			body:       `{"type":"slack","name":"test","url":"http://not-https.example"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := extension.CurrentEdition
			extension.CurrentEdition = func() extension.Edition { return tc.edition }
			defer func() { extension.CurrentEdition = original }()

			h := &AlertHandler{}

			req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCreateChannel(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantCode != "" {
				var body ErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantCode, body.Error.Code)
				assert.Equal(t, tc.wantFeature, body.Error.Feature)
				assert.Equal(t, tc.wantRequired, body.Error.RequiredEdition)
			}
		})
	}
}

// An email channel carries its recipient address in URL and is delivered over
// SMTP, so it must be exempt from the HTTPS/SSRF rule and validated as an
// address instead.
func TestHandleCreateChannel_EmailValidation(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{"valid address accepted", "alerts@example.com", http.StatusCreated},
		{"malformed address rejected", "not-an-email", http.StatusBadRequest},
		{"empty rejected", "", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &AlertHandler{channelStore: &stubChannelStore{}, broker: NewSSEBroker(logger)}

			body := `{"type":"email","name":"team","url":"` + tc.url + `"}`
			req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCreateChannel(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// HandleTestChannel — Pro channel type guard
// ---------------------------------------------------------------------------

func TestHandleTestChannel_ProTypeBlockedOnCommunity(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: "1", Type: "slack"}}
	h := &AlertHandler{channelStore: store}

	req := httptest.NewRequest("POST", "/api/v1/channels/1/test", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleTestChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
}

// ---------------------------------------------------------------------------
// HandleUpdateChannel — Pro channel type guard
// ---------------------------------------------------------------------------

func TestHandleUpdateChannel_ProTypeBlockedOnCommunity(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: "1", Type: "webhook"}}
	h := &AlertHandler{channelStore: store}

	body := `{"type":"slack"}`
	req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleUpdateChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
}

func TestHandleUpdateChannel_RetainProTypeBlockedOnCommunity(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	// Channel already has type "slack" (created under Pro, now downgraded)
	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: "1", Type: "slack"}}
	h := &AlertHandler{channelStore: store}

	body := `{"name":"renamed"}`
	req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleUpdateChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
}
