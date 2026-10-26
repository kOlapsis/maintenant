// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ratelimit"
)

// Acceptance tests of 047-audit-small-fixes, C13: at most 100 confirmation emails an hour.

const hourlyConfirmationCap = 100

type lockedLog struct {
	mu sync.Mutex
	sb strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.String()
}

func drainConfirmations(t *testing.T, mailer *fakeMailer, n int) {
	t.Helper()
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		seen[mailer.next(t).to] = true
	}
	assert.Len(t, seen, n, "each confirmation goes to its own address")
}

// FR-004, US2 scenario 2: the 101st confirmation of the hour is not sent, and the saturation is logged once.
func TestSubscribe_CapsConfirmationsAtOneHundredAnHour(t *testing.T) {
	pinEdition(t, extension.Pro)
	logs := &lockedLog{}
	mailer := &fakeMailer{sent: make(chan sentMail, 256)}
	svc := NewSubscriberService(&recordingSubscriberStore{}, mailer, "http://localhost", slog.New(slog.NewTextHandler(logs, nil)))
	ctx := context.Background()

	for i := 0; i < hourlyConfirmationCap; i++ {
		require.NoError(t, svc.Subscribe(ctx, fmt.Sprintf("visitor-%03d@example.com", i)))
	}
	drainConfirmations(t, mailer, hourlyConfirmationCap)
	assert.Equal(t, hourlyConfirmationCap, svc.ConfirmationsThisHour())

	for i := hourlyConfirmationCap; i < hourlyConfirmationCap+3; i++ {
		require.NoError(t, svc.Subscribe(ctx, fmt.Sprintf("visitor-%03d@example.com", i)), "a refused confirmation is not an error for the visitor")
	}
	mailer.none(t)
	assert.Equal(t, hourlyConfirmationCap, svc.ConfirmationsThisHour())
	assert.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "one warning per saturated hour, got:\n%s", logs.String())
}

// FR-004, edge case "plafond atteint par des adresses légitimes": the answer is the same for everyone.
func TestHandleSubscribe_AnswersAlikeOnceTheHourlyCapIsReached(t *testing.T) {
	pinEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := &fakeMailer{sent: make(chan sentMail, 256)}
	h := &Handler{
		service:     &Service{subscribers: NewSubscriberService(&recordingSubscriberStore{}, mailer, "http://localhost", logger)},
		logger:      logger,
		subscribeRL: ratelimit.New(1000, 1000, ratelimit.NewClientIPResolver(nil)),
	}

	var first *httptest.ResponseRecorder
	for i := 0; i < hourlyConfirmationCap+2; i++ {
		rec := postSubscribe(t, h, fmt.Sprintf(`{"email":"visitor-%03d@example.com"}`, i))
		require.Equal(t, http.StatusOK, rec.Code, "request %d", i+1)
		require.Contains(t, rec.Body.String(), "confirmation_sent")
		if first == nil {
			first = rec
			continue
		}
		assert.Equal(t, first.Body.String(), rec.Body.String(), "request %d must answer like the first", i+1)
	}
	drainConfirmations(t, mailer, hourlyConfirmationCap)
	mailer.none(t)
}
