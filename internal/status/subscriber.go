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

package status

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

// SubscriberService manages email subscriptions for status updates.
type SubscriberService struct {
	store  SubscriberStore
	mailer Mailer
	logger *slog.Logger

	baseURL string
}

// NewSubscriberService creates a new subscriber service.
func NewSubscriberService(store SubscriberStore, mailer Mailer, baseURL string, logger *slog.Logger) *SubscriberService {
	return &SubscriberService{
		store:   store,
		mailer:  mailer,
		logger:  logger,
		baseURL: baseURL,
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Subscribe creates a new subscriber with double opt-in.
func (s *SubscriberService) Subscribe(ctx context.Context, email string) error {
	s.logger.Info("status: new subscription request", "email", email)
	confirmToken, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate confirm token: %w", err)
	}
	unsubToken, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate unsub token: %w", err)
	}

	expires := time.Now().Add(24 * time.Hour)
	sub := &StatusSubscriber{
		Email:          email,
		Confirmed:      false,
		ConfirmToken:   &confirmToken,
		ConfirmExpires: &expires,
		UnsubToken:     unsubToken,
	}

	if _, err := s.store.CreateSubscriber(ctx, sub); err != nil {
		return fmt.Errorf("create subscriber: %w", err)
	}

	if s.mailer != nil {
		confirmURL := fmt.Sprintf("%s/status/confirm?token=%s", s.baseURL, confirmToken)
		body := fmt.Sprintf(`<html><body>
<h2>Confirm your subscription</h2>
<p>Click the link below to confirm your status page subscription:</p>
<p><a href="%s">Confirm Subscription</a></p>
<p>This link expires in 24 hours.</p>
</body></html>`, confirmURL)
		if err := s.mailer.Send(email, "Confirm your status page subscription", body); err != nil {
			s.logger.Error("failed to send confirmation email", "error", err, "email", email)
		}
	}

	return nil
}

// Confirm validates a confirmation token and activates the subscription.
func (s *SubscriberService) Confirm(ctx context.Context, token string) error {
	sub, err := s.store.GetSubscriberByToken(ctx, token)
	if err != nil || sub == nil {
		return fmt.Errorf("invalid or expired token")
	}
	if sub.ConfirmExpires != nil && sub.ConfirmExpires.Before(time.Now()) {
		return fmt.Errorf("confirmation token expired")
	}
	s.logger.Info("status: subscription confirmed", "email", sub.Email)
	return s.store.ConfirmSubscriber(ctx, sub.ID)
}

// Unsubscribe removes a subscriber by their unsubscribe token.
func (s *SubscriberService) Unsubscribe(ctx context.Context, token string) error {
	sub, err := s.store.GetSubscriberByUnsubToken(ctx, token)
	if err != nil || sub == nil {
		return fmt.Errorf("invalid unsubscribe token")
	}
	s.logger.Info("status: unsubscribed", "email", sub.Email)
	return s.store.DeleteSubscriber(ctx, sub.ID)
}

// CleanExpired removes unconfirmed subscribers older than 24 hours.
func (s *SubscriberService) CleanExpired(ctx context.Context) (int64, error) {
	return s.store.CleanExpiredUnconfirmed(ctx)
}

// Start runs periodic cleanup of expired unconfirmed subscribers.
func (s *SubscriberService) Start(ctx context.Context) {
	s.logger.Info("status: subscriber cleanup loop started")
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := s.CleanExpired(ctx)
			if err != nil {
				s.logger.Error("subscriber cleanup failed", "error", err)
			} else if deleted > 0 {
				s.logger.Info("cleaned expired subscribers", "deleted", deleted)
			}
		}
	}
}
