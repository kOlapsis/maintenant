// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license.

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/webhook"
	"github.com/stretchr/testify/assert"
)

type stubWebhookStore struct {
	created *webhook.WebhookSubscription
}

func (s *stubWebhookStore) List(context.Context) ([]*webhook.WebhookSubscription, error) {
	return nil, nil
}
func (s *stubWebhookStore) GetByID(context.Context, string) (*webhook.WebhookSubscription, error) {
	return nil, nil
}
func (s *stubWebhookStore) Create(_ context.Context, sub *webhook.WebhookSubscription) error {
	s.created = sub
	return nil
}
func (s *stubWebhookStore) Delete(context.Context, string) error { return nil }
func (s *stubWebhookStore) UpdateDeliveryStatus(context.Context, string, string, int) error {
	return nil
}
func (s *stubWebhookStore) ListActive(context.Context) ([]*webhook.WebhookSubscription, error) {
	return nil, nil
}

// A webhook subscription must never be created for an internal/private URL
// (SSRF). The only accepted destinations are public HTTPS endpoints, unless
// AllowPrivateWebhooks is explicitly enabled for local dev.
func TestHandleCreateWebhook_SSRF(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name         string
		allowPrivate bool
		url          string
		wantStatus   int
	}{
		{"http scheme rejected", false, "http://example.com/hook", http.StatusBadRequest},
		{"loopback rejected", false, "https://127.0.0.1/hook", http.StatusBadRequest},
		{"cloud imds rejected", false, "https://169.254.169.254/latest/meta-data/", http.StatusBadRequest},
		{"private ip rejected", false, "https://10.0.0.5/hook", http.StatusBadRequest},
		{"public https allowed", false, "https://8.8.8.8/hook", http.StatusCreated},
		{"dev mode allows private", true, "https://127.0.0.1/hook", http.StatusCreated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubWebhookStore{}
			h := NewWebhookHandler(store, logger, tc.allowPrivate)

			body := `{"name":"hook","url":"` + tc.url + `","event_types":["*"]}`
			req := httptest.NewRequest("POST", "/api/v1/webhooks", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCreateWebhook(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			if tc.wantStatus == http.StatusBadRequest {
				assert.Nil(t, store.created, "no subscription must be persisted for a rejected URL")
			}
		})
	}
}
