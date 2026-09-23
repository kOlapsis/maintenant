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
package alert

import (
	"strings"

	"github.com/kolapsis/maintenant/internal/extension"
)

// ChannelSuspension reports whether the running edition no longer opens channels of chType, and which edition would.
func ChannelSuspension(chType string) (extension.Edition, bool) {
	c, gated := extension.ChannelCapability(chType)
	if !gated || extension.Allows(c) {
		return "", false
	}
	return extension.MinEdition(c), true
}

// MarkSuspension fills Suspended and RequiredEdition from the running edition.
func (ch *NotificationChannel) MarkSuspension() {
	required, suspended := ChannelSuspension(ch.Type)
	ch.Suspended = suspended
	ch.RequiredEdition = string(required)
}

// SuspendedError is the refusal to deliver through a channel the running edition no longer opens.
type SuspendedError struct {
	Required extension.Edition
}

func (e *SuspendedError) Error() string {
	name := string(e.Required)
	if name != "" {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	return "suspended: requires the " + name + " edition"
}

// SuspendedChannel is how a suspended channel is listed beside the edition.
type SuspendedChannel struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	RequiredEdition string `json:"required_edition"`
}

// SuspendedChannels lists the enabled channels the running edition no longer opens.
func SuspendedChannels(channels []*NotificationChannel) []SuspendedChannel {
	out := []SuspendedChannel{}
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if required, suspended := ChannelSuspension(ch.Type); suspended {
			out = append(out, SuspendedChannel{ID: ch.ID, Name: ch.Name, Type: ch.Type, RequiredEdition: string(required)})
		}
	}
	return out
}
