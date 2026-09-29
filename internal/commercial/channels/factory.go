// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// NewChannels returns the email, Telegram, Slack and Teams senders.
func NewChannels(d extpoint.ChannelDeps) map[string]alert.ChannelSender {
	var smtp *SMTPSender
	if d.SMTP.Host != "" {
		smtp = NewSMTPSender(SMTPConfig(d.SMTP))
		d.Logger.Info("SMTP sender configured", "host", d.SMTP.Host)
	}
	return map[string]alert.ChannelSender{
		"email":    &emailSender{smtp: smtp},
		"telegram": NewTelegramSender(d.HTTPClient, TelegramAPIBase),
		"slack":    alert.NewWebhookSender(d.HTTPClient, formatSlackPayload, d.Logger),
		"teams":    alert.NewWebhookSender(d.HTTPClient, formatTeamsPayload, d.Logger),
	}
}
