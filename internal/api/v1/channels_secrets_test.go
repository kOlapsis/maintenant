// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C8, on the REST channel handlers.

var (
	insertedAt = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	updatedAt  = time.Date(2026, 10, 21, 11, 30, 0, 0, time.UTC)
)

// liveChannelStore keeps copies, like a database: what a handler does to its own object after
// writing never changes the row, and a row is only what was written.
type liveChannelStore struct {
	stubChannelStore
	rows   map[string]*alert.NotificationChannel
	nextID int
}

func newLiveChannelStore() *liveChannelStore {
	return &liveChannelStore{rows: map[string]*alert.NotificationChannel{}}
}

func (s *liveChannelStore) InsertChannel(_ context.Context, ch *alert.NotificationChannel) (string, error) {
	s.nextID++
	id := strconv.Itoa(s.nextID)
	row := *ch
	row.ID = id
	row.CreatedAt = insertedAt
	row.UpdatedAt = insertedAt
	s.rows[id] = &row
	return id, nil
}

func (s *liveChannelStore) GetChannel(_ context.Context, id string) (*alert.NotificationChannel, error) {
	row, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (s *liveChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	out := make([]*alert.NotificationChannel, 0, len(s.rows))
	for _, row := range s.rows {
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (s *liveChannelStore) UpdateChannel(_ context.Context, ch *alert.NotificationChannel) error {
	row := *ch
	row.CreatedAt = s.rows[ch.ID].CreatedAt
	row.UpdatedAt = updatedAt
	s.rows[ch.ID] = &row
	return nil
}

func (s *liveChannelStore) row(t *testing.T, id string) *alert.NotificationChannel {
	t.Helper()
	row, ok := s.rows[id]
	require.True(t, ok, "channel %s is not stored", id)
	return row
}

type channelHarness struct {
	h      *AlertHandler
	store  *liveChannelStore
	events chan SSEEvent
}

func newChannelHarness(t *testing.T) *channelHarness {
	t.Helper()
	withEditionPinned(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	broker := NewSSEBroker(logger)
	events := make(chan SSEEvent, 16)
	broker.AddObserver(events)
	t.Cleanup(func() { broker.RemoveObserver(events) })
	store := newLiveChannelStore()
	return &channelHarness{
		h: &AlertHandler{
			notifier: channelNotifier(), channelStore: store, broker: broker,
			// The destination rules have their own tests in internal/ssrf: keep these off the network.
			allowPrivateWebhooks: true,
		},
		store:  store,
		events: events,
	}
}

func (c *channelHarness) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	switch method {
	case http.MethodPost:
		c.h.HandleCreateChannel(rec, req)
	case http.MethodPut:
		req.SetPathValue("id", strings.TrimPrefix(path, "/api/v1/channels/"))
		c.h.HandleUpdateChannel(rec, req)
	case http.MethodGet:
		c.h.HandleListChannels(rec, req)
	}
	return rec
}

func (c *channelHarness) create(t *testing.T, body string) (string, map[string]any) {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/channels", body)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
	m := jsonMap(t, rec.Body.Bytes())
	id, _ := m["id"].(string)
	require.NotEmpty(t, id)
	return id, m
}

func (c *channelHarness) lastEvent(t *testing.T, eventType string) string {
	t.Helper()
	select {
	case ev := <-c.events:
		assert.Equal(t, eventType, ev.Type)
		data, err := json.Marshal(ev.Data)
		require.NoError(t, err)
		return string(data)
	case <-time.After(time.Second):
		t.Fatalf("no %s event was broadcast", eventType)
		return ""
	}
}

func jsonMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m), "body: %s", raw)
	return m
}

func listed(t *testing.T, rec *httptest.ResponseRecorder, id string) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Channels []map[string]any `json:"channels"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	for _, ch := range body.Channels {
		if ch["id"] == id {
			return ch
		}
	}
	t.Fatalf("channel %s is not listed: %s", id, rec.Body.String())
	return nil
}

const webhookWithSecret = `{"name":"ops","type":"webhook","url":"https://hooks.example.com/abc","headers":"{\"Authorization\":\"Bearer s3cr3t\",\"X-Trace\":\"1\"}"}`

// US3 scenario 1, FR-020: malformed headers are refused on create and on update, and nothing is written.
func TestChannels_MalformedHeadersAreRefused(t *testing.T) {
	malformed := []string{`"x"`, `"[1]"`, `"{\"a\":1}"`, `"{\"a\":{\"b\":\"c\"}}"`, `"{\"\":\"v\"}"`, `"{\"a\":null}"`}

	for _, headers := range malformed {
		t.Run("create "+headers, func(t *testing.T) {
			c := newChannelHarness(t)
			rec := c.do(http.MethodPost, "/api/v1/channels", `{"name":"bad","url":"https://hooks.example.com/x","headers":`+headers+`}`)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "INVALID_INPUT", body.Error.Code)
			assert.Empty(t, c.store.rows, "nothing is written")
			assert.Empty(t, c.events)
		})
	}

	for _, headers := range malformed {
		t.Run("update "+headers, func(t *testing.T) {
			c := newChannelHarness(t)
			id, _ := c.create(t, webhookWithSecret)
			rec := c.do(http.MethodPut, "/api/v1/channels/"+id, `{"headers":`+headers+`}`)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "INVALID_INPUT", body.Error.Code)
			assert.Equal(t, `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`, c.store.row(t, id).Headers, "the stored headers are untouched")
		})
	}
}

// FR-020: absent, empty and {} headers stay accepted.
func TestChannels_EmptyHeadersStayAccepted(t *testing.T) {
	for name, body := range map[string]string{
		"absent": `{"name":"a","url":"https://hooks.example.com/x"}`,
		"empty":  `{"name":"b","url":"https://hooks.example.com/x","headers":""}`,
		"object": `{"name":"c","url":"https://hooks.example.com/x","headers":"{}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newChannelHarness(t)
			_, m := c.create(t, body)
			assert.Equal(t, false, m["has_headers"])
			_, present := m["headers"]
			assert.False(t, present, "no headers key when none is stored")
		})
	}
}

// US3 scenario 2, FR-021, SC-004: every read surface masks the values and keeps the names.
func TestChannels_HeaderValuesAreMaskedOnEveryReadSurface(t *testing.T) {
	c := newChannelHarness(t)
	id, created := c.create(t, webhookWithSecret)

	masked := `{"Authorization":"***","X-Trace":"***"}`
	assert.Equal(t, masked, created["headers"])
	assert.Equal(t, true, created["has_headers"])
	assert.NotContains(t, c.lastEvent(t, event.ChannelCreated), "s3cr3t", "the SSE event is masked too")

	rec := c.do(http.MethodGet, "/api/v1/channels", "")
	assert.NotContains(t, rec.Body.String(), "s3cr3t")
	l := listed(t, rec, id)
	assert.Equal(t, masked, l["headers"])
	assert.Equal(t, true, l["has_headers"])

	rec = c.do(http.MethodPut, "/api/v1/channels/"+id, `{"name":"ops-renamed"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "s3cr3t")
	u := jsonMap(t, rec.Body.Bytes())
	assert.Equal(t, masked, u["headers"])
	assert.Equal(t, true, u["has_headers"])
	assert.NotContains(t, c.lastEvent(t, event.ChannelUpdated), "s3cr3t")

	assert.Equal(t, `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`, c.store.row(t, id).Headers, "the row keeps the real values")
}

// FR-021 with another data set: a single custom header is masked the same way.
func TestChannels_SingleHeaderIsMasked(t *testing.T) {
	c := newChannelHarness(t)
	_, created := c.create(t, `{"name":"k","url":"https://hooks.example.com/k","headers":"{\"X-Api-Key\":\"k-9f8e7d\"}"}`)

	assert.Equal(t, `{"X-Api-Key":"***"}`, created["headers"])
	assert.Equal(t, true, created["has_headers"])
}

// US3 scenario 4, FR-023, SC-004: platform URLs are cut after the host; a generic webhook keeps its URL.
func TestChannels_PlatformURLsAreTruncated(t *testing.T) {
	for _, tc := range []struct {
		typ, url, want string
		secret         bool
	}{
		{"slack", "https://hooks.slack.com/services/T000/B000/XXXXsecret", "https://hooks.slack.com", true},
		{"discord", "https://discord.com/api/webhooks/1234/abcdTOKEN", "https://discord.com", true},
		{"teams", "https://acme.webhook.office.com/webhookb2/guid/IncomingWebhook/id/key", "https://acme.webhook.office.com", true},
		{"webhook", "https://hooks.example.com/path/kept", "https://hooks.example.com/path/kept", false},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			c := newChannelHarness(t)
			id, created := c.create(t, `{"name":"n","type":"`+tc.typ+`","url":"`+tc.url+`"}`)

			assert.Equal(t, tc.want, created["url"])
			assert.Equal(t, tc.secret, created["has_url_secret"])
			assert.Equal(t, false, created["has_headers"])

			l := listed(t, c.do(http.MethodGet, "/api/v1/channels", ""), id)
			assert.Equal(t, tc.want, l["url"])
			assert.Equal(t, tc.secret, l["has_url_secret"])
			if tc.secret {
				assert.NotContains(t, c.do(http.MethodGet, "/api/v1/channels", "").Body.String(), strings.TrimPrefix(tc.url, tc.want))
			}
			assert.Equal(t, tc.url, c.store.row(t, id).URL, "the row keeps the full URL")
		})
	}
}

// Edge case: Telegram and email channels carry neither headers nor a secret URL.
func TestChannels_TelegramAndEmailHaveNoMaskedFields(t *testing.T) {
	c := newChannelHarness(t)
	_, tg := c.create(t, telegramBody("tg"))
	assert.Equal(t, false, tg["has_headers"])
	assert.Equal(t, false, tg["has_url_secret"])
	assert.Equal(t, "-1001234567890", tg["url"])

	_, mail := c.create(t, `{"name":"mail","type":"email","url":"ops@example.com"}`)
	assert.Equal(t, false, mail["has_headers"])
	assert.Equal(t, false, mail["has_url_secret"])
	assert.Equal(t, "ops@example.com", mail["url"])
}

// US3 scenario 5, FR-022, FR-023, SC-005: an update without headers nor url keeps both, and the next
// delivery still carries the stored headers; headers: {} clears them.
func TestChannels_UpdateWithoutHeadersOrURLKeepsThem(t *testing.T) {
	var got http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	c := newChannelHarness(t)
	id, _ := c.create(t, `{"name":"ops","type":"webhook","url":"`+target.URL+`/hook","headers":"{\"Authorization\":\"Bearer s3cr3t\"}"}`)

	rec := c.do(http.MethodPut, "/api/v1/channels/"+id, `{"name":"ops-2","enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	renamed := jsonMap(t, rec.Body.Bytes())
	assert.Equal(t, true, renamed["has_headers"], "the kept headers are still reported")
	assert.Equal(t, `{"Authorization":"***"}`, renamed["headers"])
	row := c.store.row(t, id)
	assert.Equal(t, `{"Authorization":"Bearer s3cr3t"}`, row.Headers)
	assert.Equal(t, target.URL+"/hook", row.URL)
	assert.Equal(t, "ops-2", row.Name)

	_, err := c.h.notifier.SendTestWebhook(context.Background(), row)
	require.NoError(t, err)
	assert.Equal(t, "Bearer s3cr3t", got.Get("Authorization"), "the next delivery uses the stored headers")

	rec = c.do(http.MethodPut, "/api/v1/channels/"+id, `{"headers":"{}"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, false, jsonMap(t, rec.Body.Bytes())["has_headers"])
	row = c.store.row(t, id)
	_, err = c.h.notifier.SendTestWebhook(context.Background(), row)
	require.NoError(t, err)
	assert.Empty(t, got.Get("Authorization"), "headers: {} clears the stored headers")
}

// FR-022: a new headers value replaces the old one; FR-023: a Slack URL survives a rename.
func TestChannels_NewHeadersReplaceAndSlackURLSurvivesARename(t *testing.T) {
	c := newChannelHarness(t)
	id, _ := c.create(t, `{"name":"ops","type":"slack","url":"https://hooks.slack.com/services/T000/B000/XXXXsecret","headers":"{\"X-Trace\":\"1\"}"}`)

	rec := c.do(http.MethodPut, "/api/v1/channels/"+id, `{"name":"ops-2","headers":"{\"X-Other\":\"2\"}"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	row := c.store.row(t, id)
	assert.Equal(t, `{"X-Other":"2"}`, row.Headers)
	assert.Equal(t, "https://hooks.slack.com/services/T000/B000/XXXXsecret", row.URL)
	assert.Equal(t, `{"X-Other":"***"}`, jsonMap(t, rec.Body.Bytes())["headers"])
}

// Edge case "mise à jour avec url pour un canal Slack": the new URL is stored and returned truncated.
func TestChannels_UpdatingASlackURLReturnsItTruncated(t *testing.T) {
	c := newChannelHarness(t)
	id, _ := c.create(t, `{"name":"ops","type":"slack","url":"https://hooks.slack.com/services/T000/B000/OLDsecret"}`)

	rec := c.do(http.MethodPut, "/api/v1/channels/"+id, `{"url":"https://hooks.slack.com/services/T111/B111/NEWsecret"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	m := jsonMap(t, rec.Body.Bytes())
	assert.Equal(t, "https://hooks.slack.com", m["url"])
	assert.Equal(t, true, m["has_url_secret"])
	assert.NotContains(t, rec.Body.String(), "NEWsecret")
	assert.Equal(t, "https://hooks.slack.com/services/T111/B111/NEWsecret", c.store.row(t, id).URL)
}

// US3 scenario 7, FR-024: POST and PUT answer with the stored row, dates included.
func TestChannels_ResponsesCarryTheStoredDates(t *testing.T) {
	c := newChannelHarness(t)
	id, created := c.create(t, `{"name":"ops","url":"https://hooks.example.com/x"}`)

	assert.Equal(t, insertedAt.Format(time.RFC3339), created["created_at"])
	assert.Equal(t, insertedAt.Format(time.RFC3339), created["updated_at"])

	rec := c.do(http.MethodPut, "/api/v1/channels/"+id, `{"name":"ops-2"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	updated := jsonMap(t, rec.Body.Bytes())
	assert.Equal(t, insertedAt.Format(time.RFC3339), updated["created_at"])
	assert.Equal(t, updatedAt.Format(time.RFC3339), updated["updated_at"])
}
