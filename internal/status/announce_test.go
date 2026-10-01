// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
)

type notifyCall struct {
	ctxErr           error
	subject, message string
}

type recordingNotifier struct {
	calls chan notifyCall
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{calls: make(chan notifyCall, 16)}
}

func (n *recordingNotifier) NotifyAll(ctx context.Context, subject, message string) {
	n.calls <- notifyCall{ctxErr: ctx.Err(), subject: subject, message: message}
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

type recordingBroadcaster struct {
	mu       sync.Mutex
	events   []string
	payloads []any
}

func (b *recordingBroadcaster) broadcast(eventType string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, eventType)
	b.payloads = append(b.payloads, data)
}

func (b *recordingBroadcaster) payload(t *testing.T, eventType string) map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, e := range b.events {
		if e == eventType {
			return b.payloads[i].(map[string]any)
		}
	}
	t.Fatalf("no %s event among %v", eventType, b.events)
	return nil
}

func (b *recordingBroadcaster) all() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.events...)
}

func newAnnouncingService(t *testing.T, mailer Mailer) (*Service, *recordingNotifier, *recordingBroadcaster) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := &recordingBroadcaster{}
	svc := NewService(Deps{
		Components:        emptyComponentStore{},
		Logger:            logger,
		PublicBroadcaster: b.broadcast,
		Subscribers:       NewSubscriberService(&recordingSubscriberStore{}, mailer, "http://localhost", logger),
	})
	n := newRecordingNotifier()
	svc.SetSubscriberNotifier(n)
	return svc, n, b
}

func newTwoStreamService() (*Service, *recordingBroadcaster, *recordingBroadcaster) {
	public, admin := &recordingBroadcaster{}, &recordingBroadcaster{}
	svc := NewService(Deps{
		Components:        emptyComponentStore{},
		Logger:            discardLogger(),
		PublicBroadcaster: public.broadcast,
		AdminBroadcaster:  admin.broadcast,
	})
	return svc, public, admin
}

func TestHiddenComponentChangeStaysOffThePublicStream(t *testing.T) {
	svc, public, admin := newTwoStreamService()

	svc.BroadcastComponentChange(context.Background(), &Component{
		ID: "db", DisplayName: "Internal DB", CompositionMode: CompositionMatchAll, MatchAllType: "container",
	})

	if got := public.all(); len(got) != 0 {
		t.Fatalf("the public stream heard %v about a hidden component", got)
	}
	if got := admin.all(); len(got) != 1 || got[0] != event.StatusComponentChanged {
		t.Fatalf("dashboard events %v, want one %s", got, event.StatusComponentChanged)
	}
}

func TestVisibleComponentChangeReachesBothStreams(t *testing.T) {
	svc, public, admin := newTwoStreamService()

	svc.BroadcastComponentChange(context.Background(), &Component{
		ID: "api", DisplayName: "API", CompositionMode: CompositionMatchAll, MatchAllType: "endpoint", Visible: true,
	})

	if got := public.all(); len(got) != 2 || got[0] != event.StatusComponentChanged || got[1] != event.StatusGlobalChanged {
		t.Fatalf("public events %v, want %s then %s", got, event.StatusComponentChanged, event.StatusGlobalChanged)
	}
	if got := admin.all(); len(got) == 0 || got[0] != event.StatusComponentChanged {
		t.Fatalf("dashboard events %v, want %s first", got, event.StatusComponentChanged)
	}
}

func TestAnnounceComponentChangeReachesThePublicPageOnlyWhenItConcernsIt(t *testing.T) {
	svc, public, admin := newTwoStreamService()

	svc.AnnounceComponentChange(context.Background(), event.StatusComponentUpdated, "hidden", false)
	if got := public.all(); len(got) != 0 {
		t.Fatalf("the public stream heard %v about a hidden component", got)
	}
	if got := admin.all(); len(got) != 1 || got[0] != event.StatusComponentUpdated {
		t.Fatalf("dashboard events %v, want one %s", got, event.StatusComponentUpdated)
	}

	svc.AnnounceComponentChange(context.Background(), event.StatusComponentUpdated, "api", true)
	if got := public.all(); len(got) != 2 || got[0] != event.StatusComponentUpdated || got[1] != event.StatusGlobalChanged {
		t.Fatalf("public events %v, want %s then %s", got, event.StatusComponentUpdated, event.StatusGlobalChanged)
	}
}

func TestAnnounceIncidentEmailsSubscribersOnce(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, n, b := newAnnouncingService(t, newFakeMailer())

	inc := &Incident{
		ID: "inc-1", Title: "Database down", Severity: SeverityMajor, Status: IncidentInvestigating,
		Components: []IncidentCompRef{{ID: "c1", Name: "API", Visible: true}, {ID: "c2", Name: "Web", Visible: true}},
	}
	svc.AnnounceIncident(context.Background(), inc, "We are investigating.")

	call := n.next(t)
	if call.subject != "[major] Database down" {
		t.Fatalf("subject %q", call.subject)
	}
	for _, want := range []string{"Database down", "Severity: major", "Status: investigating", "Affected components: API, Web", "We are investigating."} {
		if !strings.Contains(call.message, want) {
			t.Fatalf("message %q lacks %q", call.message, want)
		}
	}
	n.none(t)
	if got := b.all(); len(got) != 1 || got[0] != event.StatusIncidentCreated {
		t.Fatalf("events %v, want one %s", got, event.StatusIncidentCreated)
	}
}

func TestAnnounceIncidentNamesOnlyVisibleComponentsInPublic(t *testing.T) {
	pinEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	public, admin := &recordingBroadcaster{}, &recordingBroadcaster{}
	svc := NewService(Deps{
		Components:        emptyComponentStore{},
		Logger:            logger,
		PublicBroadcaster: public.broadcast,
		AdminBroadcaster:  admin.broadcast,
		Subscribers:       NewSubscriberService(&recordingSubscriberStore{}, newFakeMailer(), "http://localhost", logger),
	})
	n := newRecordingNotifier()
	svc.SetSubscriberNotifier(n)

	svc.AnnounceIncident(context.Background(), &Incident{
		ID: "inc-1", Title: "Database down", Severity: SeverityMajor, Status: IncidentInvestigating,
		Components: []IncidentCompRef{{ID: "c1", Name: "API", Visible: true}, {ID: "c2", Name: "Internal DB"}},
	}, "")

	if got := public.payload(t, event.StatusIncidentCreated)["components"]; !reflect.DeepEqual(got, []string{"API"}) {
		t.Fatalf("public components %v, want only the visible one", got)
	}
	if got := admin.payload(t, event.StatusIncidentCreated)["components"]; !reflect.DeepEqual(got, []string{"API", "Internal DB"}) {
		t.Fatalf("dashboard components %v, want every one", got)
	}
	if mail := n.next(t).message; !strings.Contains(mail, "Affected components: API\n") || strings.Contains(mail, "Internal DB") {
		t.Fatalf("mail %q must name the visible component only", mail)
	}
}

func TestAnnounceIncidentUpdateResolvingSendsOnlyTheResolution(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, n, b := newAnnouncingService(t, newFakeMailer())

	inc := &Incident{ID: "inc-1", Title: "Database down", Status: IncidentInvestigating}
	svc.AnnounceIncidentUpdate(context.Background(), inc, &IncidentUpdate{IncidentID: "inc-1", Status: IncidentResolved, Message: "Back to normal."})

	call := n.next(t)
	if call.subject != "Resolved: Database down" {
		t.Fatalf("subject %q", call.subject)
	}
	if !strings.Contains(call.message, "Back to normal.") {
		t.Fatalf("message %q lacks the update", call.message)
	}
	n.none(t)
	if got := b.all(); len(got) != 1 || got[0] != event.StatusIncidentResolved {
		t.Fatalf("events %v, want one %s", got, event.StatusIncidentResolved)
	}
}

func TestAnnounceIncidentUpdateOnAnOpenIncident(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, n, b := newAnnouncingService(t, newFakeMailer())

	inc := &Incident{ID: "inc-1", Title: "Database down", Status: IncidentInvestigating}
	svc.AnnounceIncidentUpdate(context.Background(), inc, &IncidentUpdate{IncidentID: "inc-1", Status: "monitoring", Message: "Fix deployed."})

	call := n.next(t)
	if call.subject != "Update: Database down" {
		t.Fatalf("subject %q", call.subject)
	}
	if !strings.Contains(call.message, "Status: monitoring") || !strings.Contains(call.message, "Fix deployed.") {
		t.Fatalf("message %q", call.message)
	}
	n.none(t)
	if got := b.all(); len(got) != 1 || got[0] != event.StatusIncidentUpdated {
		t.Fatalf("events %v, want one %s", got, event.StatusIncidentUpdated)
	}
}

func TestAnnounceIncidentUpdateAfterResolutionIsAnUpdate(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, n, _ := newAnnouncingService(t, newFakeMailer())

	inc := &Incident{ID: "inc-1", Title: "Database down", Status: IncidentResolved}
	svc.AnnounceIncidentUpdate(context.Background(), inc, &IncidentUpdate{IncidentID: "inc-1", Status: IncidentResolved, Message: "Post-mortem published."})

	if call := n.next(t); call.subject != "Update: Database down" {
		t.Fatalf("subject %q: an incident resolves only once", call.subject)
	}
	n.none(t)
}

func TestNotifySubscribersOutlivesTheRequest(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, n, _ := newAnnouncingService(t, newFakeMailer())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.NotifySubscribers(ctx, "subject", "message")

	if call := n.next(t); call.ctxErr != nil {
		t.Fatalf("the notifier got a cancelled context: %v", call.ctxErr)
	}
}

func TestNotifySubscribersSilentWhileSubscriptionsAreClosed(t *testing.T) {
	cases := []struct {
		name    string
		edition extension.Edition
		mailer  Mailer
	}{
		{"no SMTP server", extension.Pro, nil},
		{"edition without subscribers", extension.Personal, newFakeMailer()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinEdition(t, tc.edition)
			svc, n, b := newAnnouncingService(t, tc.mailer)

			svc.AnnounceIncident(context.Background(), &Incident{ID: "inc-1", Title: "x", Severity: SeverityMinor}, "")

			n.none(t)
			if len(b.all()) != 1 {
				t.Fatal("the public page must still hear about the incident")
			}
		})
	}
}

type tokenSubscriberStore struct {
	recordingSubscriberStore
	confirmToken string
}

func (s *tokenSubscriberStore) UpsertPendingSubscriber(ctx context.Context, sub *StatusSubscriber) (bool, error) {
	s.confirmToken = *sub.ConfirmToken
	return s.recordingSubscriberStore.UpsertPendingSubscriber(ctx, sub)
}

func TestSubscribeEmailsTheConfirmationLink(t *testing.T) {
	pinEdition(t, extension.Pro)
	store := &tokenSubscriberStore{}
	mailer := newFakeMailer()
	svc := NewSubscriberService(store, mailer, "https://status.example.com/", slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.Subscribe(context.Background(), "visitor@example.com"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	mail := mailer.next(t)
	if mail.to != "visitor@example.com" {
		t.Fatalf("mail to %s, want the visitor", mail.to)
	}
	link := regexp.MustCompile(`https://status\.example\.com/status/confirm\?token=([0-9a-f]{64})`).FindStringSubmatch(mail.body)
	if link == nil {
		t.Fatalf("body %q holds no confirmation link", mail.body)
	}
	if link[1] != store.confirmToken {
		t.Fatalf("link token %q is not the stored token %q", link[1], store.confirmToken)
	}
	mailer.none(t)
}

func TestSubscribeSendsNothingToAConfirmedAddress(t *testing.T) {
	pinEdition(t, extension.Pro)
	store := &recordingSubscriberStore{confirmed: map[string]bool{"visitor@example.com": true}}
	mailer := newFakeMailer()
	svc := NewSubscriberService(store, mailer, "https://status.example.com", slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.Subscribe(context.Background(), "visitor@example.com"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	mailer.none(t)
}

func TestSubscribeRefusedWhileSubscriptionsAreClosed(t *testing.T) {
	pinEdition(t, extension.Pro)
	store := &recordingSubscriberStore{}
	svc := NewSubscriberService(store, nil, "http://localhost", slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.Subscribe(context.Background(), "visitor@example.com"); !errors.Is(err, ErrSubscriptionsDisabled) {
		t.Fatalf("got %v, want ErrSubscriptionsDisabled", err)
	}
	if len(store.created) != 0 {
		t.Fatalf("the store was written to: %v", store.created)
	}
}
