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
