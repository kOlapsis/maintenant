// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant
package statuspage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/status"
)

// SubscriberNotifier emails status updates to confirmed subscribers.
type SubscriberNotifier struct {
	store   status.SubscriberStore
	mailer  status.Mailer
	baseURL string
	logger  *slog.Logger
}

// NewSubscriberNotifier returns a notifier sending through mailer, which may be nil.
func NewSubscriberNotifier(store status.SubscriberStore, mailer status.Mailer, baseURL string, logger *slog.Logger) *SubscriberNotifier {
	return &SubscriberNotifier{store: store, mailer: mailer, baseURL: baseURL, logger: logger}
}

// NotifyAll sends an incident notification to all confirmed subscribers.
func (s *SubscriberNotifier) NotifyAll(ctx context.Context, subject, message string) {
	if s.mailer == nil {
		s.logger.Debug("status: SMTP not configured, skipping notification")
		return
	}

	subs, err := s.store.ListConfirmedSubscribers(ctx)
	if err != nil {
		s.logger.Error("failed to list subscribers for notification", "error", err)
		return
	}

	s.logger.Debug("status: sending notifications", "count", len(subs), "subject", subject)

	for _, sub := range subs {
		unsubURL := fmt.Sprintf("%s/status/unsubscribe?token=%s", s.baseURL, sub.UnsubToken)
		body := fmt.Sprintf(`<html><body>
<h2>Status Update</h2>
<p>%s</p>
<hr>
<p><small><a href="%s">Unsubscribe</a></small></p>
</body></html>`, message, unsubURL)

		if err := s.mailer.Send(sub.Email, subject, body); err != nil {
			s.logger.Error("failed to send notification", "error", err, "email", sub.Email)
		}
	}
}
