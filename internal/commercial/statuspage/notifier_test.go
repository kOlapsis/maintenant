// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/channels"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/status"
)

type confirmedSubscribers struct {
	status.SubscriberStore
	subs []status.StatusSubscriber
}

func (s *confirmedSubscribers) ListConfirmedSubscribers(context.Context) ([]status.StatusSubscriber, error) {
	return s.subs, nil
}

type sentMail struct {
	to, subject, body string
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to: to, subject: subject, body: body})
	return nil
}

func TestNotifyAllMailsEachConfirmedSubscriberWithItsOwnUnsubscribeLink(t *testing.T) {
	store := &confirmedSubscribers{subs: []status.StatusSubscriber{
		{Email: "a@example.com", UnsubToken: "tok-a"},
		{Email: "b@example.com", UnsubToken: "tok-b"},
	}}
	mailer := &fakeMailer{}
	n := NewSubscriberNotifier(store, mailer, "https://status.example.com/", discardLogger())

	n.NotifyAll(context.Background(), "[major] Database down", "Database down\n\nWe are investigating.\n")

	require.Len(t, mailer.sent, 2)
	for i, sub := range store.subs {
		got := mailer.sent[i]
		assert.Equal(t, sub.Email, got.to)
		assert.Equal(t, "[major] Database down", got.subject)
		assert.Equal(t, "Database down\n\nWe are investigating.\n\n-- \nUnsubscribe: https://status.example.com/status/unsubscribe?token="+sub.UnsubToken+"\n", got.body)
	}
}

func TestNewStatusPageMailsThroughTheEnvironmentSMTP(t *testing.T) {
	deps := extpoint.StatusPageDeps{Logger: discardLogger(), BaseURL: "https://status.example.com"}

	without := NewStatusPage(deps)
	assert.Nil(t, without.Mailer, "no SMTP host, no mailer")
	assert.Nil(t, without.Notifier, "no SMTP host, no notifier")

	deps.SMTP = extpoint.SMTPConfig{Host: "smtp.example.com", Port: "587", From: "status@example.com"}
	with := NewStatusPage(deps)
	assert.IsType(t, &channels.SMTPSender{}, with.Mailer, "the status page reuses the SMTP client of the email channel")
	assert.NotNil(t, with.Notifier)
}

type notifyCall struct {
	subject, message string
}

type recordingNotifier struct {
	calls chan notifyCall
}

func (n *recordingNotifier) NotifyAll(_ context.Context, subject, message string) {
	n.calls <- notifyCall{subject: subject, message: message}
}

func (n *recordingNotifier) next(t *testing.T) notifyCall {
	t.Helper()
	select {
	case c := <-n.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("no notification was sent")
		return notifyCall{}
	}
}

func (n *recordingNotifier) none(t *testing.T) {
	t.Helper()
	select {
	case c := <-n.calls:
		t.Fatalf("unexpected notification %q", c.subject)
	case <-time.After(100 * time.Millisecond):
	}
}

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = original })
}

func newNotifyingService(t *testing.T, cs status.ComponentStore, is status.IncidentStore) (*status.Service, *recordingNotifier) {
	t.Helper()
	pinEdition(t, extension.Pro)
	svc := newTestService(cs, is)
	svc.SetSubscriberService(status.NewSubscriberService(&confirmedSubscribers{}, &fakeMailer{}, "https://status.example.com", discardLogger()))
	n := &recordingNotifier{calls: make(chan notifyCall, 8)}
	svc.SetSubscriberNotifier(n)
	return svc, n
}

func TestAutoIncidentMailsOnOpeningAndResolutionOnly(t *testing.T) {
	comp := makeExplicitComponent("endpoint", "ep-5")
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	is := &mockIncidentStore{createIncidentID: "inc-9"}
	svc, n := newNotifyingService(t, cs, is)
	monitor := status.StatusMajorOutage
	svc.SetMonitorStatusProvider(func(context.Context, string, string) string { return monitor })

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))
	opened := n.next(t)
	assert.Equal(t, "[critical] API Gateway - connection refused", opened.subject)
	assert.Contains(t, opened.message, "Affected components: API Gateway")
	assert.Contains(t, opened.message, "connection refused")
	n.none(t)

	existing := &status.Incident{ID: "inc-9", Title: "API Gateway - connection refused", Status: status.IncidentInvestigating}
	is.mu.Lock()
	is.activeByComponent = map[string]*status.Incident{comp.ID: existing}
	is.mu.Unlock()

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))
	n.none(t)

	monitor = status.StatusOperational
	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", true))
	resolved := n.next(t)
	assert.Equal(t, "Resolved: API Gateway - connection refused", resolved.subject)
	n.none(t)
}
