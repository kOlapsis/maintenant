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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/outbound"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestOutboundHeartbeatStore_CRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewOutboundHeartbeatStore(db)

	created := time.Unix(1_700_000_000, 0)
	o := &outbound.OutboundHeartbeat{
		ID:              uid.New(),
		Name:            "upstream",
		URL:             "https://mnt.example/ping/abc",
		IntervalSeconds: 60,
		Enabled:         true,
		CreatedAt:       created,
		UpdatedAt:       created,
	}
	require.NoError(t, s.Create(ctx, o))

	got, err := s.Get(ctx, o.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, o.Name, got.Name)
	assert.Equal(t, o.URL, got.URL)
	assert.Equal(t, 60, got.IntervalSeconds)
	assert.True(t, got.Enabled)
	assert.Nil(t, got.LastSentAt)
	assert.Nil(t, got.LastStatusCode)
	assert.Nil(t, got.LastError)
	assert.Equal(t, created.Unix(), got.CreatedAt.Unix())

	o.Name = "renamed"
	o.Enabled = false
	o.UpdatedAt = created.Add(time.Minute)
	found, err := s.Update(ctx, o)
	require.NoError(t, err)
	assert.True(t, found)

	got, err = s.Get(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.False(t, got.Enabled)
	assert.Equal(t, created.Add(time.Minute).Unix(), got.UpdatedAt.Unix())

	list, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	found, err = s.Delete(ctx, o.ID)
	require.NoError(t, err)
	assert.True(t, found)

	got, err = s.Get(ctx, o.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	var n int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM outbound_heartbeats WHERE id = ?", o.ID).Scan(&n))
	assert.Zero(t, n)
}

func TestOutboundHeartbeatStore_UnknownID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewOutboundHeartbeatStore(db)

	found, err := s.Update(ctx, &outbound.OutboundHeartbeat{ID: uid.New(), Name: "x", URL: "http://x", IntervalSeconds: 60})
	require.NoError(t, err)
	assert.False(t, found)

	found, err = s.Delete(ctx, uid.New())
	require.NoError(t, err)
	assert.False(t, found)
}

func TestOutboundHeartbeatStore_RecordSend(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewOutboundHeartbeatStore(db)

	now := time.Unix(1_700_000_000, 0)
	o := &outbound.OutboundHeartbeat{ID: uid.New(), Name: "a", URL: "http://a", IntervalSeconds: 30,
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.Create(ctx, o))

	code := 503
	msg := "unexpected status 503 Service Unavailable"
	require.NoError(t, s.RecordSend(ctx, o.ID, outbound.SendResult{SentAt: now.Add(time.Minute), StatusCode: &code, Error: &msg}))

	got, err := s.Get(ctx, o.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastSentAt)
	assert.Equal(t, now.Add(time.Minute).Unix(), got.LastSentAt.Unix())
	require.NotNil(t, got.LastStatusCode)
	assert.Equal(t, 503, *got.LastStatusCode)
	require.NotNil(t, got.LastError)
	assert.Equal(t, msg, *got.LastError)

	ok := 200
	require.NoError(t, s.RecordSend(ctx, o.ID, outbound.SendResult{SentAt: now.Add(2 * time.Minute), StatusCode: &ok}))
	got, err = s.Get(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, 200, *got.LastStatusCode)
	assert.Nil(t, got.LastError, "a success clears the previous error")
	assert.Equal(t, now.Unix(), got.UpdatedAt.Unix(), "a send does not touch updated_at")
}
