// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/resource"
)

func TestAlertConfigWrites_SettingsAndStateAreIndependent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-alert-split", "")
	rs := NewResourceStore(db)

	require.NoError(t, rs.UpdateAlertSettings(ctx, cid, 70, 80, true))
	alertedAt := time.Now().Add(-time.Minute).Truncate(time.Second)
	require.NoError(t, rs.UpdateAlertState(ctx, cid, resource.AlertStateBoth, 4, 5, &alertedAt))

	cfg, err := rs.GetAlertConfig(ctx, cid)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 70.0, cfg.CPUThreshold)
	assert.Equal(t, 80.0, cfg.MemThreshold)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, resource.AlertStateBoth, cfg.AlertState)
	assert.Equal(t, 4, cfg.CPUConsecutiveBreaches)
	assert.Equal(t, 5, cfg.MemConsecutiveBreaches)
	require.NotNil(t, cfg.LastAlertedAt)
	assert.Equal(t, alertedAt.Unix(), cfg.LastAlertedAt.Unix())

	require.NoError(t, rs.UpdateAlertSettings(ctx, cid, 10, 20, false))
	cfg, err = rs.GetAlertConfig(ctx, cid)
	require.NoError(t, err)
	assert.Equal(t, 10.0, cfg.CPUThreshold)
	assert.Equal(t, 20.0, cfg.MemThreshold)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, resource.AlertStateBoth, cfg.AlertState, "writing settings leaves the state alone")
	assert.Equal(t, 4, cfg.CPUConsecutiveBreaches)
	assert.Equal(t, 5, cfg.MemConsecutiveBreaches)
	require.NotNil(t, cfg.LastAlertedAt)

	require.NoError(t, rs.UpdateAlertState(ctx, cid, resource.AlertStateNormal, 0, 0, nil))
	cfg, err = rs.GetAlertConfig(ctx, cid)
	require.NoError(t, err)
	assert.Equal(t, 10.0, cfg.CPUThreshold, "writing state leaves the settings alone")
	assert.Equal(t, 20.0, cfg.MemThreshold)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, resource.AlertStateNormal, cfg.AlertState)
	assert.Nil(t, cfg.LastAlertedAt)
}

func TestUpdateAlertState_DoesNotCreateAConfig(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-alert-nocfg", "")
	rs := NewResourceStore(db)

	require.NoError(t, rs.UpdateAlertState(ctx, cid, resource.AlertStateCPU, 2, 0, nil))

	cfg, err := rs.GetAlertConfig(ctx, cid)
	require.NoError(t, err)
	assert.Nil(t, cfg)
}
