// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"errors"
	"testing"
)

func TestChannelPatchApply(t *testing.T) {
	tests := []struct {
		name      string
		patch     ChannelPatch
		wantErr   string
		wantField string
		check     func(t *testing.T, ch *NotificationChannel)
	}{
		{
			name:  "fields applied",
			patch: ChannelPatch{Name: new("n"), Type: new("slack"), URL: new("u"), Headers: new(`{"A":"b"}`), Secret: new("s"), Enabled: new(false)},
			check: func(t *testing.T, ch *NotificationChannel) {
				if ch.Name != "n" || ch.Type != "slack" || ch.URL != "u" || ch.Headers != `{"A":"b"}` || ch.Secret != "s" || ch.Enabled {
					t.Fatalf("unexpected channel %+v", ch)
				}
			},
		},
		{
			name:  "absent secret keeps stored",
			patch: ChannelPatch{Name: new("n")},
			check: func(t *testing.T, ch *NotificationChannel) {
				if ch.Secret != "stored" || !ch.Enabled {
					t.Fatalf("unexpected channel %+v", ch)
				}
			},
		},
		{
			name:      "invalid headers",
			patch:     ChannelPatch{Headers: new(`[1]`)},
			wantErr:   "field=headers: " + errInvalidHeaders.Error(),
			wantField: "headers",
		},
		{
			name:      "empty secret refused",
			patch:     ChannelPatch{Secret: new("")},
			wantErr:   "field=secret: cannot be cleared; delete the channel instead",
			wantField: "secret",
		},
		{
			name:  "config null becomes empty",
			patch: ChannelPatch{Config: new(" null ")},
			check: func(t *testing.T, ch *NotificationChannel) {
				if ch.Config != "" {
					t.Fatalf("Config = %q", ch.Config)
				}
			},
		},
		{
			name:  "config trimmed",
			patch: ChannelPatch{Config: new(` {"a":"b"} `)},
			check: func(t *testing.T, ch *NotificationChannel) {
				if ch.Config != `{"a":"b"}` {
					t.Fatalf("Config = %q", ch.Config)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := &NotificationChannel{Secret: "stored", Config: "old", Enabled: true}
			err := tc.patch.Apply(ch)
			if tc.wantErr != "" {
				var fe *ChannelFieldError
				if !errors.As(err, &fe) || fe.Field != tc.wantField || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, ch)
		})
	}
}

func TestChannelPatchTouches(t *testing.T) {
	if (&ChannelPatch{Name: new("n"), Enabled: new(true)}).TouchesCredentials() {
		t.Fatal("name and enabled do not touch credentials")
	}
	if !(&ChannelPatch{Secret: new("s")}).TouchesCredentials() || !(&ChannelPatch{Config: new("")}).TouchesCredentials() {
		t.Fatal("secret and config touch credentials")
	}
	if (&ChannelPatch{Name: new("n"), Secret: new("s")}).TouchesDestination() {
		t.Fatal("name and secret do not touch the destination")
	}
	if !(&ChannelPatch{URL: new("u")}).TouchesDestination() || !(&ChannelPatch{Type: new("t")}).TouchesDestination() {
		t.Fatal("url and type touch the destination")
	}
}
