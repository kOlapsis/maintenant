// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import "strings"

// ChannelPatch is a partial update of a notification channel: a nil field keeps the stored value.
type ChannelPatch struct {
	Name    *string
	Type    *string
	URL     *string
	Headers *string
	Secret  *string
	Config  *string
	Enabled *bool
}

// ChannelFieldError names the patch field that failed validation.
type ChannelFieldError struct {
	Field string
	Msg   string
}

func (e *ChannelFieldError) Error() string { return "field=" + e.Field + ": " + e.Msg }

// NormalizeConfig trims a channel config and turns a JSON null into the empty string.
func NormalizeConfig(config string) string {
	config = strings.TrimSpace(config)
	if config == "null" {
		return ""
	}
	return config
}

// Extends reports whether the patch does more than switch the channel off, which never needs the edition (FR-001d).
func (p *ChannelPatch) Extends() bool {
	return p.Name != nil || p.Type != nil || p.URL != nil ||
		p.Headers != nil || p.Secret != nil || p.Config != nil ||
		(p.Enabled != nil && *p.Enabled)
}

// TouchesCredentials reports whether the patch changes the secret or the config.
func (p *ChannelPatch) TouchesCredentials() bool { return p.Secret != nil || p.Config != nil }

// TouchesDestination reports whether the patch changes the URL or the type.
func (p *ChannelPatch) TouchesDestination() bool { return p.URL != nil || p.Type != nil }

// Apply writes the patch onto ch, refusing invalid headers and an empty secret; an absent secret keeps the stored one (FR-006).
func (p *ChannelPatch) Apply(ch *NotificationChannel) error {
	if p.Name != nil {
		ch.Name = *p.Name
	}
	if p.Type != nil {
		ch.Type = *p.Type
	}
	if p.URL != nil {
		ch.URL = *p.URL
	}
	if p.Headers != nil {
		if _, err := ParseHeaders(*p.Headers); err != nil {
			return &ChannelFieldError{Field: "headers", Msg: err.Error()}
		}
		ch.Headers = *p.Headers
	}
	if p.Secret != nil {
		if *p.Secret == "" {
			return &ChannelFieldError{Field: "secret", Msg: "cannot be cleared; delete the channel instead"}
		}
		ch.Secret = *p.Secret
	}
	if p.Config != nil {
		ch.Config = NormalizeConfig(*p.Config)
	}
	if p.Enabled != nil {
		ch.Enabled = *p.Enabled
	}
	return nil
}
