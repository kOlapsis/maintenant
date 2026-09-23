// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

type telegramSender struct {
	client  *http.Client
	apiBase string
}

// NewTelegramSender returns the Telegram sender, calling apiBase with client.
func NewTelegramSender(client *http.Client, apiBase string) alert.ChannelSender {
	return &telegramSender{client: client, apiBase: apiBase}
}

func (s *telegramSender) Ready() error { return nil }

func (s *telegramSender) Send(ctx context.Context, ch *alert.NotificationChannel, eventType string, a *alert.Alert) error {
	return SendTelegram(ctx, s.client, s.apiBase, ch, BuildTelegramMessage(eventType, a))
}

func (s *telegramSender) RetryDelay(wait time.Duration, lastErr error) time.Duration {
	return telegramBackoff(wait, lastErr)
}

func (s *telegramSender) FailureMessage(lastErr error) string { return lastErr.Error() }

func (s *telegramSender) SendTest(ctx context.Context, ch *alert.NotificationChannel, testAlert *alert.Alert) (int, error) {
	if err := SendTelegram(ctx, s.client, s.apiBase, ch, BuildTelegramMessage("test", testAlert)); err != nil {
		return 0, err
	}
	return 200, nil
}

func (s *telegramSender) ValidateDestination(destination string) error {
	return ValidateChatID(destination)
}

func (s *telegramSender) ValidateCredentials(secret, config string) error {
	if err := ValidateBotToken(secret); err != nil {
		return err
	}
	cfg, err := ParseTelegramConfig(config)
	if err != nil {
		return errors.New("config must be a JSON object")
	}
	return ValidateThreadID(cfg.ThreadID)
}

// telegramBackoff returns the product's own backoff, raised to the delay Telegram asked for when it asked for one: never
// retrying sooner than Telegram allows, never replacing our policy with theirs.
func telegramBackoff(wait time.Duration, lastErr error) time.Duration {
	var rateLimit *TelegramRateLimitError
	if errors.As(lastErr, &rateLimit) && rateLimit.RetryAfter > wait {
		return rateLimit.RetryAfter
	}
	return wait
}
