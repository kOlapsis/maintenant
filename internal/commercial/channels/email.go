// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kolapsis/maintenant/internal/alert"
)

const (
	defaultSubjectLabel = "maintenant"
	maxSubjectLabelLen  = 64
)

type emailConfig struct {
	SubjectLabel string `json:"subject_label"`
}

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
	return s.smtp.Send(ctx, ch.URL, formatEmailSubject(subjectLabel(ch), eventType, a.Message), formatEmailBody(eventType, a))
}

func (s *emailSender) RetryDelay(wait time.Duration, _ error) time.Duration { return wait }

func (s *emailSender) FailureMessage(error) string { return "email delivery failed after retries" }

func (s *emailSender) SendTest(ctx context.Context, ch *alert.NotificationChannel, _ *alert.Alert) (int, error) {
	err := s.smtp.Send(ctx, ch.URL, formatEmailSubject(subjectLabel(ch), "test", "Test notification"), "This is a test notification from maintenant.\n\nIf you received this email, your alert channel is configured correctly.")
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

func (s *emailSender) ValidateCredentials(_, config string) error {
	if strings.TrimSpace(config) == "" {
		return nil
	}
	var cfg emailConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return errors.New("config must be a JSON object")
	}
	if strings.IndexFunc(cfg.SubjectLabel, unicode.IsControl) >= 0 {
		return errors.New("subject label must not contain line breaks or control characters")
	}
	if utf8.RuneCountInString(strings.TrimSpace(cfg.SubjectLabel)) > maxSubjectLabelLen {
		return fmt.Errorf("subject label must be at most %d characters", maxSubjectLabelLen)
	}
	return nil
}

func subjectLabel(ch *alert.NotificationChannel) string {
	var cfg emailConfig
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return defaultSubjectLabel
	}
	if label := strings.TrimSpace(cfg.SubjectLabel); label != "" {
		return label
	}
	return defaultSubjectLabel
}

func formatEmailSubject(label, eventType, message string) string {
	prefix := "ALERT"
	if strings.Contains(eventType, "resolved") {
		prefix = "RESOLVED"
	} else if eventType == "test" {
		prefix = "TEST"
	}
	return fmt.Sprintf("[%s] %s: %s", label, prefix, message)
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
