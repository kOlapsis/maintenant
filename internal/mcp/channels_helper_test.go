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
package mcp

import (
	"io"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial/channels"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// channelNotifier returns a notifier holding the production channel senders, validators included.
func channelNotifier() *alert.Notifier {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	n := alert.NewNotifier(nil, logger, true)
	for chType, s := range channels.NewChannels(extpoint.ChannelDeps{HTTPClient: n.HTTPClient(), Logger: logger}) {
		n.RegisterChannel(chType, s)
	}
	return n
}
