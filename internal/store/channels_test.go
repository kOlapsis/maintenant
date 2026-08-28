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

package store

import (
	"context"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A channel carrying a credential and per-type settings must come back with
// both, on either engine. The columns are new in migration 30, so this is also
// what proves the migration reached the table the store writes to.
func TestChannelStore_SecretAndConfigRoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := NewChannelStore(db)
	ctx := context.Background()

	id, err := s.InsertChannel(ctx, &alert.NotificationChannel{
		Name:    "telegram-oncall",
		Type:    "telegram",
		URL:     "-1001234567890",
		Secret:  "8123456789:AAF-token",
		Config:  `{"thread_id":"42"}`,
		Enabled: true,
	})
	require.NoError(t, err)

	got, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "8123456789:AAF-token", got.Secret)
	assert.Equal(t, `{"thread_id":"42"}`, got.Config)
	assert.True(t, got.HasSecret, "HasSecret is derived at scan time")

	list, err := s.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "8123456789:AAF-token", list[0].Secret,
		"the list path scans the same columns as the single-row path")
	assert.True(t, list[0].HasSecret)
}

// The five older channel types leave both columns NULL. Scanning must give the
// zero value, not an error, and HasSecret must stay false.
func TestChannelStore_ExistingTypesKeepBothColumnsNull(t *testing.T) {
	db := openTestDB(t)
	s := NewChannelStore(db)
	ctx := context.Background()

	id, err := s.InsertChannel(ctx, &alert.NotificationChannel{
		Name:    "generic-webhook",
		Type:    "webhook",
		URL:     "https://example.com/hook",
		Enabled: true,
	})
	require.NoError(t, err)

	got, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Secret)
	assert.Empty(t, got.Config)
	assert.False(t, got.HasSecret)
}

// An update that carries the secret forward keeps it; one that carries an empty
// secret clears it. The store applies what it is handed — deciding whether an
// absent token means "keep" is the handler's job, and its test covers that.
func TestChannelStore_UpdateAppliesTheSecretItIsGiven(t *testing.T) {
	db := openTestDB(t)
	s := NewChannelStore(db)
	ctx := context.Background()

	id, err := s.InsertChannel(ctx, &alert.NotificationChannel{
		Name: "telegram-oncall", Type: "telegram", URL: "-100123",
		Secret: "8123456789:AAF-token", Enabled: true,
	})
	require.NoError(t, err)

	ch, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	ch.Enabled = false
	require.NoError(t, s.UpdateChannel(ctx, ch))

	got, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "8123456789:AAF-token", got.Secret, "disabling must not drop the token")

	got.Secret = ""
	require.NoError(t, s.UpdateChannel(ctx, got))
	after, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, after.Secret)
	assert.False(t, after.HasSecret)
}

// Deletion is not gated by anything and must stay that way: it is one of the
// two ways out for an operator whose licence expired (FR-001d).
func TestChannelStore_DeleteRemovesTheChannel(t *testing.T) {
	db := openTestDB(t)
	s := NewChannelStore(db)
	ctx := context.Background()

	id, err := s.InsertChannel(ctx, &alert.NotificationChannel{
		Name: "telegram-oncall", Type: "telegram", URL: "-100123",
		Secret: "8123456789:AAF-token", Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, s.DeleteChannel(ctx, id))

	got, err := s.GetChannel(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, got)
}
