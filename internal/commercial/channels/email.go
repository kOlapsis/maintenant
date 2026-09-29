// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

type emailSender struct {
	smtp *SMTPSender
}

func (s *emailSender) Ready() error {
	if s.smtp == nil {
		return errors.New("SMTP not configured")
	}
	return nil
}

func (s *emailSender) Send(ctx context.Context, ch *alert.NotificationChannel, eventType string, a *alert.Alert) error {
	return s.smtp.Send(ctx, ch.URL, formatEmailSubject(eventType, a), formatEmailBody(eventType, a))
}

func (s *emailSender) RetryDelay(wait time.Duration, _ error) time.Duration { return wait }

func (s *emailSender) FailureMessage(error) string { return "email delivery failed after retries" }

func (s *emailSender) SendTest(ctx context.Context, ch *alert.NotificationChannel, _ *alert.Alert) (int, error) {
	err := s.smtp.Send(ctx, ch.URL, "maintenant Test Notification", "This is a test notification from maintenant.\n\nIf you received this email, your alert channel is configured correctly.")
	if err != nil {
		return 0, err
	}
	return 200, nil
}

func (s *emailSender) ValidateDestination(destination string) error {
	if _, err := mail.ParseAddress(destination); err != nil {
		return errors.New("invalid email address")
	}
	return nil
}

func (s *emailSender) ValidateCredentials(string, string) error { return nil }

func formatEmailSubject(eventType string, a *alert.Alert) string {
	prefix := "ALERT"
	if strings.Contains(eventType, "resolved") {
		prefix = "RESOLVED"
	} else if eventType == "test" {
		prefix = "TEST"
	}
	return fmt.Sprintf("[maintenant] %s: %s", prefix, a.Message)
}

func formatEmailBody(eventType string, a *alert.Alert) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Event: %s\n", eventType)
	fmt.Fprintf(&b, "Source: %s\n", a.Source)
	fmt.Fprintf(&b, "Severity: %s\n", a.Severity)
	fmt.Fprintf(&b, "Entity: %s (%s)\n", a.EntityName, a.EntityType)
	fmt.Fprintf(&b, "Message: %s\n", a.Message)
	fmt.Fprintf(&b, "Time: %s\n", a.FiredAt.UTC().Format(time.RFC3339))

	if a.Source == "update" {
		if details := alert.ParseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				fmt.Fprintf(&b, "\nUpdate command:\n  %s\n", cmd)
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				fmt.Fprintf(&b, "\nRollback command:\n  %s\n", cmd)
			}
		}
	}

	return b.String()
}
