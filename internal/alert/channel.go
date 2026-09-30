// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ChannelSender delivers alerts to one type of notification channel; the notifier owns retries and delivery records.
type ChannelSender interface {
	// Ready reports why the sender cannot deliver at all, before any attempt is made.
	Ready() error
	Send(ctx context.Context, ch *NotificationChannel, eventType string, a *Alert) error
	// RetryDelay may lengthen the notifier's backoff after lastErr, never shorten it.
	RetryDelay(wait time.Duration, lastErr error) time.Duration
	// FailureMessage is what the delivery record keeps once every attempt failed.
	FailureMessage(lastErr error) string
	SendTest(ctx context.Context, ch *NotificationChannel, testAlert *Alert) (int, error)
}

// ChannelValidator checks a channel's destination and credentials before it is stored.
type ChannelValidator interface {
	ValidateDestination(destination string) error
	ValidateCredentials(secret, config string) error
}

// PayloadFormatter renders the JSON body a webhook-style channel receives.
type PayloadFormatter func(eventType string, a *Alert) ([]byte, error)

type webhookSender struct {
	client *http.Client
	format PayloadFormatter
	logger *slog.Logger
}

// NewWebhookSender returns a sender that POSTs the formatted payload to the channel URL.
func NewWebhookSender(client *http.Client, format PayloadFormatter, logger *slog.Logger) ChannelSender {
	return &webhookSender{client: client, format: format, logger: logger}
}

func (s *webhookSender) Ready() error { return nil }

func (s *webhookSender) RetryDelay(wait time.Duration, _ error) time.Duration { return wait }

func (s *webhookSender) FailureMessage(lastErr error) string { return lastErr.Error() }

func (s *webhookSender) Send(ctx context.Context, ch *NotificationChannel, eventType string, a *Alert) error {
	body, err := s.format(eventType, a)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return s.post(ctx, ch, body)
}

func (s *webhookSender) post(ctx context.Context, ch *NotificationChannel, body []byte) error {
	s.logger.Debug("alert notifier: sending webhook",
		"url", ch.URL,
		"channel_type", ch.Type,
	)
	req, err := s.request(ctx, ch, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx response: %d", resp.StatusCode)
	}
	return nil
}

func (s *webhookSender) SendTest(ctx context.Context, ch *NotificationChannel, testAlert *Alert) (int, error) {
	body, err := s.format("test", testAlert)
	if err != nil {
		return 0, err
	}
	return s.sendBody(ctx, ch, body)
}

// sendBody posts body once and reports the HTTP status, with the start of the answer when it is not a 2xx.
func (s *webhookSender) sendBody(ctx context.Context, ch *NotificationChannel, body []byte) (int, error) {
	req, err := s.request(ctx, ch, body)
	if err != nil {
		return 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return resp.StatusCode, nil
}

func (s *webhookSender) request(ctx context.Context, ch *NotificationChannel, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if ch.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(ch.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	return req, nil
}
