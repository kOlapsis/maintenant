// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kolapsis/maintenant/internal/status"
)

// SubscriberNotifier emails status updates to confirmed subscribers.
type SubscriberNotifier struct {
	store   status.SubscriberStore
	mailer  status.Mailer
	baseURL string
	logger  *slog.Logger
}

// NewSubscriberNotifier returns a notifier sending through mailer.
func NewSubscriberNotifier(store status.SubscriberStore, mailer status.Mailer, baseURL string, logger *slog.Logger) *SubscriberNotifier {
	return &SubscriberNotifier{store: store, mailer: mailer, baseURL: strings.TrimRight(baseURL, "/"), logger: logger}
}

// NotifyAll emails message, followed by a personal unsubscribe link, to every confirmed subscriber.
func (s *SubscriberNotifier) NotifyAll(ctx context.Context, subject, message string) {
	subs, err := s.store.ListConfirmedSubscribers(ctx)
	if err != nil {
		s.logger.Error("failed to list subscribers for notification", "error", err)
		return
	}

	s.logger.Debug("status: sending notifications", "count", len(subs), "subject", subject)

	text := strings.TrimRight(message, "\n")
	for _, sub := range subs {
		body := text + "\n\n-- \nUnsubscribe: " + s.baseURL + "/status/unsubscribe?token=" + sub.UnsubToken + "\n"
		if err := s.mailer.Send(ctx, sub.Email, subject, body); err != nil {
			s.logger.Error("failed to send notification", "error", err, "email", sub.Email)
		}
	}
}
