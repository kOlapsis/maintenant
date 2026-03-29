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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type stubChannelStore struct {
	ch *alert.NotificationChannel
}

func (s *stubChannelStore) InsertChannel(_ context.Context, ch *alert.NotificationChannel) (int64, error) {
	return 1, nil
}
func (s *stubChannelStore) GetChannel(_ context.Context, _ int64) (*alert.NotificationChannel, error) {
	return s.ch, nil
}
func (s *stubChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (s *stubChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (s *stubChannelStore) DeleteChannel(_ context.Context, _ int64) error { return nil }
func (s *stubChannelStore) GetChannelHealth(_ context.Context, _ int64) (string, error) {
	return "ok", nil
}
func (s *stubChannelStore) InsertRoutingRule(_ context.Context, _ *alert.RoutingRule) (int64, error) {
	return 1, nil
}
func (s *stubChannelStore) DeleteRoutingRule(_ context.Context, _ int64) error { return nil }
func (s *stubChannelStore) ListRoutingRulesByChannel(_ context.Context, _ int64) ([]alert.RoutingRule, error) {
	return nil, nil
}
func (s *stubChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (int64, error) {
	return 1, nil
}
func (s *stubChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (s *stubChannelStore) ListDeliveriesByAlert(_ context.Context, _ int64) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// HandleCreateChannel — Pro channel type guard
// ---------------------------------------------------------------------------

func TestHandleCreateChannel_TypeGating(t *testing.T) {
	tests := []struct {
		name       string
		edition    extension.Edition
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "community + slack blocked",
			edition:    extension.Community,
			body:       `{"type":"slack","name":"test","url":"https://example.com"}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "PRO_REQUIRED",
		},
		{
			name:       "community + teams blocked",
			edition:    extension.Community,
			body:       `{"type":"teams","name":"test","url":"https://example.com"}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "PRO_REQUIRED",
		},
		{
			name:       "community + email blocked",
			edition:    extension.Community,
			body:       `{"type":"email","name":"test","url":"https://example.com"}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "PRO_REQUIRED",
		},
		{
			name:    "community + webhook passes type check",
			edition: extension.Community,
			// http:// URL fails HTTPS scheme check → 400, not 403
			body:       `{"type":"webhook","name":"test","url":"http://not-https.example"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:    "enterprise + slack passes type check",
			edition: extension.Enterprise,
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
				assert.Contains(t, rec.Body.String(), tc.wantCode)
			}
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

	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: 1, Type: "slack"}}
	h := &AlertHandler{channelStore: store}

	req := httptest.NewRequest("POST", "/api/v1/channels/1/test", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleTestChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "PRO_REQUIRED")
}

// ---------------------------------------------------------------------------
// HandleUpdateChannel — Pro channel type guard
// ---------------------------------------------------------------------------

func TestHandleUpdateChannel_ProTypeBlockedOnCommunity(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: 1, Type: "webhook"}}
	h := &AlertHandler{channelStore: store}

	body := `{"type":"slack"}`
	req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleUpdateChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "PRO_REQUIRED")
}

func TestHandleUpdateChannel_RetainProTypeBlockedOnCommunity(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	// Channel already has type "slack" (created under Enterprise, now downgraded)
	store := &stubChannelStore{ch: &alert.NotificationChannel{ID: 1, Type: "slack"}}
	h := &AlertHandler{channelStore: store}

	body := `{"name":"renamed"}`
	req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleUpdateChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "PRO_REQUIRED")
}
