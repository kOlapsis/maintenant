// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial/channels"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// Acceptance tests of 049-email-subject-prefix on the channels API: real router, real store, real email sender
// talking to an SMTP server that keeps the decoded subject of every message.

func smtpSubjects(t *testing.T) (extpoint.SMTPConfig, func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	var mu sync.Mutex
	var subjects []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				reply := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
				reply("220 stub")
				inData, inHeader := false, false
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					switch {
					case inData && line == ".":
						inData = false
						reply("250 ok")
					case inData:
						if line == "" {
							inHeader = false
						}
						if raw, ok := strings.CutPrefix(line, "Subject: "); ok && inHeader {
							decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
							if err != nil {
								decoded = "undecodable: " + raw
							}
							mu.Lock()
							subjects = append(subjects, decoded)
							mu.Unlock()
						}
					case strings.EqualFold(line, "DATA"):
						inData, inHeader = true, true
						reply("354 go")
					case strings.EqualFold(line, "QUIT"):
						reply("221 bye")
						return
					default:
						reply("250 ok")
					}
				}
			}(conn)
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	return extpoint.SMTPConfig{Host: host, Port: port, From: "maintenant@example.com", AllowPlaintext: true}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), subjects...)
	}
}

type subjectFixture struct {
	handler  http.Handler
	channels *store.ChannelStoreImpl
	notifier *alert.Notifier
	subjects func() []string
}

// The email channel opens at Personal, and the label asks for nothing more.
func newSubjectFixture(t *testing.T) subjectFixture {
	t.Helper()
	withEditionPinned(t, extension.Personal)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	smtp, subjects := smtpSubjects(t)

	n := alert.NewNotifier(nil, logger, true)
	for chType, s := range channels.NewChannels(extpoint.ChannelDeps{HTTPClient: n.HTTPClient(), SMTP: smtp, Logger: logger}) {
		n.RegisterChannel(chType, s)
	}
	cs := store.NewChannelStore(db)
	r := NewRouter(HandlerDeps{
		Logger:       logger,
		Broker:       NewSSEBroker(logger),
		AlertStore:   store.NewAlertStore(db),
		ChannelStore: cs,
		SilenceStore: store.NewSilenceStore(db),
		Notifier:     n,
	})
	return subjectFixture{handler: r.Handler(), channels: cs, notifier: n, subjects: subjects}
}

func (f subjectFixture) call(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(t, err)
		rd = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func emailChannelBody(name, label string) map[string]any {
	return map[string]any{"name": name, "type": "email", "url": "ops@example.com", "config": map[string]string{"subject_label": label}}
}

type channelAnswer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Config string `json:"config"`
}

func labelOf(t *testing.T, config string) string {
	t.Helper()
	if config == "" {
		return ""
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(config), &m), "config %q", config)
	label, _ := m["subject_label"].(string)
	return label
}

func (f subjectFixture) create(t *testing.T, name, label string) channelAnswer {
	t.Helper()
	rec := f.call(t, http.MethodPost, "/api/v1/channels", emailChannelBody(name, label))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var ch channelAnswer
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ch))
	require.NotEmpty(t, ch.ID)
	return ch
}

// list reads the channels back through GET /api/v1/channels.
func (f subjectFixture) list(t *testing.T) []channelAnswer {
	t.Helper()
	rec := f.call(t, http.MethodGet, "/api/v1/channels", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Channels []channelAnswer `json:"channels"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Channels
}

func (f subjectFixture) read(t *testing.T, id string) channelAnswer {
	t.Helper()
	for _, ch := range f.list(t) {
		if ch.ID == id {
			return ch
		}
	}
	t.Fatalf("channel %s is not listed", id)
	return channelAnswer{}
}

func (f subjectFixture) nextSubject(t *testing.T, send func()) string {
	t.Helper()
	before := len(f.subjects())
	send()
	got := f.subjects()
	require.Len(t, got, before+1, "one email is sent")
	return got[before]
}

// testSubject presses Test on the saved channel and returns the subject received.
func (f subjectFixture) testSubject(t *testing.T, id string) string {
	t.Helper()
	return f.nextSubject(t, func() {
		rec := f.call(t, http.MethodPost, "/api/v1/channels/"+id+"/test", nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), `"status":"delivered"`)
	})
}

// alertSubject sends an alert the way the engine and the escalation runner do, with the channel read from the store.
func (f subjectFixture) alertSubject(t *testing.T, id, message string) string {
	t.Helper()
	return f.nextSubject(t, func() {
		ch, err := f.channels.GetChannel(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, ch)
		a := &alert.Alert{
			ID: "a-" + message, Source: "endpoint", AlertType: "endpoint_down", Severity: alert.SeverityCritical,
			Status: alert.StatusActive, Message: message, EntityType: "endpoint", EntityName: "api.example.com",
			FiredAt: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC),
		}
		require.NoError(t, f.notifier.SendNow(context.Background(), a, ch))
	})
}

// US2 scenario 3, FR-001, FR-005, contract api.md: the wizard saves the channel with its label, then tests it; the
// answer carries config as stored and the test subject carries the label. 64 non-latin characters fit.
func TestEmailSubjectAPI_CreatedChannelTestsWithItsLabel(t *testing.T) {
	for _, label := range []string{"staging", "recette-東京", strings.Repeat("東", 64)} {
		t.Run(label, func(t *testing.T) {
			f := newSubjectFixture(t)

			ch := f.create(t, "ops mail", label)

			assert.Equal(t, label, labelOf(t, ch.Config), "the answer carries config as stored")
			assert.Equal(t, label, labelOf(t, f.read(t, ch.ID).Config), "the list carries it too")
			assert.Equal(t, "["+label+"] TEST: Test notification", f.testSubject(t, ch.ID))
		})
	}
}

// US2 scenario 4, US1 scenarios 1 and 5, contract api.md (config replaces the whole object, {} or an empty label
// clears it): every send after a save uses the saved label.
func TestEmailSubjectAPI_EditedLabelIsUsedByTheNextTestAndAlert(t *testing.T) {
	f := newSubjectFixture(t)
	id := f.create(t, "ops mail", "staging").ID

	for _, step := range []struct{ config, prefix string }{
		{`{"subject_label":"prod-paris"}`, "[prod-paris]"},
		{`{"subject_label":"prod-lyon"}`, "[prod-lyon]"},
		{`{"subject_label":""}`, "[maintenant]"},
		{`{"subject_label":"prod-paris"}`, "[prod-paris]"},
		{`{}`, "[maintenant]"},
	} {
		rec := f.call(t, http.MethodPut, "/api/v1/channels/"+id, `{"config":`+step.config+`}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		assert.Equal(t, step.prefix+" TEST: Test notification", f.testSubject(t, id), "test after config %s", step.config)
		assert.Equal(t, step.prefix+" ALERT: Disk 93% full", f.alertSubject(t, id, "Disk 93% full"), "alert after config %s", step.config)
	}
}

// FR-009, edge "a client that does not send the field": an update that does not mention config keeps the label.
func TestEmailSubjectAPI_UpdateWithoutConfigKeepsTheLabel(t *testing.T) {
	f := newSubjectFixture(t)
	id := f.create(t, "ops mail", "prod-paris").ID

	for _, body := range []string{
		`{"name":"ops mail 2"}`,
		`{"url":"oncall@example.com"}`,
		`{"enabled":true}`,
		`{"name":"ops mail 3","url":"ops@example.com","enabled":true}`,
	} {
		rec := f.call(t, http.MethodPut, "/api/v1/channels/"+id, body)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		assert.Equal(t, "prod-paris", labelOf(t, f.read(t, id).Config), "after %s", body)
		assert.Equal(t, "[prod-paris] TEST: Test notification", f.testSubject(t, id), "after %s", body)
	}
}

// US3 scenarios 1 to 4, FR-007, contract api.md errors: an invalid label is refused with 400 VALIDATION_ERROR and a
// message that names the label and the problem, on create and on update; a refused update changes nothing.
func TestEmailSubjectAPI_InvalidLabelIsRefusedAndChangesNothing(t *testing.T) {
	for name, tc := range map[string]struct{ label, says string }{
		"65 characters":          {strings.Repeat("a", 65), `64`},
		"65 accented characters": {strings.Repeat("é", 65), `64`},
		"line feed":              {"prod\nparis", `(?i)control`},
		"header injection":       {"prod\r\nBcc: evil@example.com", `(?i)control`},
		"tab":                    {"prod\tparis", `(?i)control`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSubjectFixture(t)

			refused := func(rec *httptest.ResponseRecorder) {
				t.Helper()
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				var body ErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "VALIDATION_ERROR", body.Error.Code)
				assert.Regexp(t, tc.says, body.Error.Message)
				assert.Regexp(t, `(?i)subject.?label`, body.Error.Message)
			}

			refused(f.call(t, http.MethodPost, "/api/v1/channels", emailChannelBody("refused mail", tc.label)))
			assert.Empty(t, f.list(t), "a refused create stores nothing")

			id := f.create(t, "ops mail", "prod-paris").ID
			refused(f.call(t, http.MethodPut, "/api/v1/channels/"+id, map[string]any{
				"name":   "renamed",
				"config": map[string]string{"subject_label": tc.label},
			}))

			stored := f.read(t, id)
			assert.Equal(t, "prod-paris", labelOf(t, stored.Config), "the label on file is unchanged")
			assert.Equal(t, "ops mail", stored.Name, "nothing else changed either")
			assert.Equal(t, "[prod-paris] TEST: Test notification", f.testSubject(t, id))
		})
	}
}
