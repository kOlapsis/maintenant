// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectingServer answers /hook with status and a Location to /hook/, and records what /hook/ receives.
func redirectingServer(t *testing.T, status int) (srv *httptest.Server, got chan string) {
	t.Helper()
	got = make(chan string, 10)
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/hook/")
		w.WriteHeader(status)
	})
	mux.HandleFunc("/hook/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- r.Method + " " + string(body)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, got
}

func TestNotifier_ARedirectThatDropsTheBodyIsAFailedDelivery(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, got := redirectingServer(t, status)
			n := newTestNotifier()

			err := n.webhook.post(context.Background(), &NotificationChannel{Type: "webhook", URL: srv.URL + "/hook"}, []byte(`{"a":1}`))

			var statusErr *HTTPStatusError
			require.ErrorAs(t, err, &statusErr)
			assert.Equal(t, status, statusErr.Status)
			assert.False(t, isPermanent(err), "a redirect is retried like a 5xx")
			assert.Empty(t, got, "the redirect target must not be called without the body")
		})
	}
}

func TestNotifier_ARedirectThatKeepsTheBodyIsFollowed(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, got := redirectingServer(t, status)
			n := newTestNotifier()

			require.NoError(t, n.webhook.post(context.Background(), &NotificationChannel{Type: "webhook", URL: srv.URL + "/hook"}, []byte(`{"a":1}`)))

			require.Len(t, got, 1)
			assert.Equal(t, `POST {"a":1}`, <-got)
		})
	}
}
