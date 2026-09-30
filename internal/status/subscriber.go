// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

// ErrSubscriptionsDisabled is returned when no mailer is configured or the running edition does not open subscribers.
var ErrSubscriptionsDisabled = errors.New("email subscriptions are not enabled")

// SubscriberService manages email subscriptions for status updates.
type SubscriberService struct {
	store  SubscriberStore
	mailer Mailer
	logger *slog.Logger

	baseURL string
}

// NewSubscriberService creates a subscriber service; a nil mailer keeps subscriptions disabled.
func NewSubscriberService(store SubscriberStore, mailer Mailer, baseURL string, logger *slog.Logger) *SubscriberService {
	return &SubscriberService{
		store:   store,
		mailer:  mailer,
		logger:  logger,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// Enabled reports whether a mailer is configured and the running edition opens subscribers.
func (s *SubscriberService) Enabled() bool {
	return s != nil && s.mailer != nil && extension.Allows(extension.CapSubscribers)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Subscribe records a pending subscription and, unless the address is already confirmed, emails a fresh confirmation link in the background.
func (s *SubscriberService) Subscribe(ctx context.Context, email string) error {
	if !s.Enabled() {
		return ErrSubscriptionsDisabled
	}
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
	issued, err := s.store.UpsertPendingSubscriber(ctx, &StatusSubscriber{
		Email:          email,
		ConfirmToken:   &confirmToken,
		ConfirmExpires: &expires,
		UnsubToken:     unsubToken,
	})
	if err != nil {
		return fmt.Errorf("record subscriber: %w", err)
	}
	if issued {
		go s.sendConfirmation(context.WithoutCancel(ctx), email, confirmToken)
	}
	return nil
}

func (s *SubscriberService) sendConfirmation(ctx context.Context, email, token string) {
	confirmURL := fmt.Sprintf("%s/status/confirm?token=%s", s.baseURL, token)
	body := fmt.Sprintf("Confirm your subscription to status updates by opening this link:\n\n%s\n\n"+
		"The link expires in 24 hours. If you did not ask for this, ignore this email.\n", confirmURL)
	if err := s.mailer.Send(ctx, email, "Confirm your status page subscription", body); err != nil {
		s.logger.Error("failed to send confirmation email", "error", err, "email", email)
	}
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
