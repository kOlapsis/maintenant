// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type discardMailer struct{}

func (discardMailer) Send(context.Context, string, string, string) error { return nil }

type brokenSubscriberStore struct{ status.SubscriberStore }

var errDriver = errors.New("database is locked (5) (SQLITE_BUSY)")

func (brokenSubscriberStore) ConfirmSubscriber(context.Context, string) error { return errDriver }
func (brokenSubscriberStore) DeleteSubscriber(context.Context, string) error  { return errDriver }

func newLinkHandler(t *testing.T, wrap func(status.SubscriberStore) status.SubscriberStore) (*status.Handler, *store.SubscriberStoreImpl) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = original })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	subs := store.NewSubscriberStore(db)
	svc := status.NewService(status.Deps{
		Components:  store.NewStatusComponentStore(db),
		Logger:      logger,
		Subscribers: status.NewSubscriberService(wrap(subs), discardMailer{}, "http://localhost", logger),
	})
	return status.NewHandler(svc, nil, logger, nil, ""), subs
}

func addPending(t *testing.T, subs *store.SubscriberStoreImpl) {
	t.Helper()
	confirm, expires := "confirm-token", time.Now().Add(time.Hour)
	issued, err := subs.UpsertPendingSubscriber(context.Background(), &status.StatusSubscriber{
		Email: "visitor@example.com", ConfirmToken: &confirm, ConfirmExpires: &expires, UnsubToken: "unsub-token",
	})
	require.NoError(t, err)
	require.True(t, issued)
}

func get(h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSubscriptionLinks_OpenedTwiceAnswerANeutralPage(t *testing.T) {
	h, subs := newLinkHandler(t, func(s status.SubscriberStore) status.SubscriberStore { return s })
	addPending(t, subs)

	for _, link := range []struct {
		name    string
		handler http.HandlerFunc
		target  string
	}{
		{"confirm", h.HandleConfirm, "/status/confirm?token=confirm-token"},
		{"unsubscribe", h.HandleUnsubscribe, "/status/unsubscribe?token=unsub-token"},
	} {
		t.Run(link.name, func(t *testing.T) {
			require.Equal(t, http.StatusOK, get(link.handler, link.target).Code)

			second := get(link.handler, link.target)
			assert.Equal(t, http.StatusOK, second.Code)
			assert.NotContains(t, second.Body.String(), "Error")
		})
	}
}

func TestSubscriptionLinks_StoreFailureDoesNotReachTheVisitor(t *testing.T) {
	h, subs := newLinkHandler(t, func(s status.SubscriberStore) status.SubscriberStore { return brokenSubscriberStore{s} })
	addPending(t, subs)

	for _, link := range []struct {
		name    string
		handler http.HandlerFunc
		target  string
	}{
		{"confirm", h.HandleConfirm, "/status/confirm?token=confirm-token"},
		{"unsubscribe", h.HandleUnsubscribe, "/status/unsubscribe?token=unsub-token"},
	} {
		t.Run(link.name, func(t *testing.T) {
			rec := get(link.handler, link.target)
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.NotContains(t, rec.Body.String(), "SQLITE_BUSY")
		})
	}
}
