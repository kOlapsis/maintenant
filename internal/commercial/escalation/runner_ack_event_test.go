// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/event"
)

type webhookInbox struct {
	mu       sync.Mutex
	payloads []alert.WebhookPayload
}

func (in *webhookInbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p alert.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	in.mu.Lock()
	in.payloads = append(in.payloads, p)
	in.mu.Unlock()
}

func (in *webhookInbox) received() []alert.WebhookPayload {
	in.mu.Lock()
	defer in.mu.Unlock()
	return append([]alert.WebhookPayload(nil), in.payloads...)
}

func TestAcknowledgmentReachesAWebhookAsAnAcknowledgment(t *testing.T) {
	inbox := &webhookInbox{}
	srv := httptest.NewServer(inbox)
	t.Cleanup(srv.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := newDBHarness(t, alert.NewNotifier(newChannelStoreMock(), logger, true))
	ch := h.addChannel("hook", srv.URL, true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}},
		esc.Level{Order: 1, DelaySeconds: 600, ChannelIDs: []string{ch}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)
	h.cycle()
	h.settle()
	require.Len(t, inbox.received(), 1)

	at := h.clock()
	acked, err := h.alerts.AcknowledgeAlert(h.ctx, a.ID, "alice", at)
	require.NoError(t, err)
	require.True(t, acked)
	require.NoError(t, h.runner.OnAlertAcknowledged(h.ctx, a.ID, alert.Acknowledgment{By: "alice", At: at}))
	h.settle()

	got := inbox.received()
	require.Len(t, got, 2)
	assert.Equal(t, event.AlertFired, got[0].Event)
	notice := got[1]
	assert.NotEqual(t, event.AlertResolved, notice.Event, "an acknowledgment is not a resolution")
	assert.Equal(t, event.AlertAcknowledged, notice.Event)
	assert.Equal(t, alert.StatusActive, notice.Alert["status"], "the incident is still going on")
	assert.Equal(t, "alice", notice.Alert["acknowledged_by"])
	assert.Contains(t, notice.Alert["message"], "Acknowledged by alice")
}
