// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
