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

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/wneessen/go-mail"
)

// SmtpClient wraps go-mail to send emails.
type SmtpClient struct {
	config status.SmtpConfig
}

// NewSmtpClient creates a new SMTP client from the given config.
func NewSmtpClient(config status.SmtpConfig) *SmtpClient {
	return &SmtpClient{config: config}
}

// Send sends an HTML email to the given recipient.
func (c *SmtpClient) Send(to, subject, htmlBody string) error {
	if !c.config.Configured {
		return fmt.Errorf("SMTP not configured")
	}

	msg := mail.NewMsg()
	if err := msg.From(c.config.FromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("invalid to address: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)

	if c.config.FromName != "" {
		msg.SetGenHeader("From", fmt.Sprintf("%s <%s>", c.config.FromName, c.config.FromAddress))
	}

	opts := []mail.Option{
		mail.WithPort(c.config.Port),
	}

	switch c.config.TLSPolicy {
	case status.TLSMandatory:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	case status.TLSNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))
	}

	if c.config.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(c.config.Username),
			mail.WithPassword(c.config.Password),
		)
	}

	client, err := mail.NewClient(c.config.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create mail client: %w", err)
	}

	return client.DialAndSend(msg)
}

// TestConnection verifies SMTP connectivity by dialing the server.
func (c *SmtpClient) TestConnection() error {
	if !c.config.Configured {
		return fmt.Errorf("SMTP not configured")
	}

	opts := []mail.Option{
		mail.WithPort(c.config.Port),
	}

	switch c.config.TLSPolicy {
	case status.TLSMandatory:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	case status.TLSNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))
	}

	if c.config.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(c.config.Username),
			mail.WithPassword(c.config.Password),
		)
	}

	client, err := mail.NewClient(c.config.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create mail client: %w", err)
	}

	return client.DialWithContext(context.TODO())
}
