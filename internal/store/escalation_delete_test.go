// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
)

func seedPolicyWithActiveRun(t *testing.T, db *DB) (policyID, runID string) {
	t.Helper()
	ctx := context.Background()
	s := NewEscalationStore(db)
	policyID, err := s.InsertPolicy(ctx, makeTestPolicy("on-call", true))
	require.NoError(t, err)
	alertID := seedAlert(t, NewAlertStore(db), "e1", alert.StatusActive, time.Now(), nil)
	next := time.Now().Add(time.Minute)
	runID, err = s.InsertRun(ctx, &escalation.Run{
		PolicyID: &policyID, PolicySnapshotJSON: "{}", AlertID: alertID, Status: escalation.RunStatusActive,
		LastExecutedLevelIndex: -1, StartedAt: time.Now(), NextActionAt: &next,
	})
	require.NoError(t, err)
	return policyID, runID
}

func TestEscalationStore_DeletePolicyStopsItsRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewEscalationStore(db)
	policyID, runID := seedPolicyWithActiveRun(t, db)

	require.NoError(t, s.DeletePolicy(ctx, policyID))

	run, err := s.SelectRun(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, escalation.RunStatusStoppedByPolicyDelete, run.Status)
	assert.Nil(t, run.PolicyID)
}

func TestEscalationStore_DeletePolicyKeepsTheRunsWhenTheDeleteFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewEscalationStore(db)
	policyID, runID := seedPolicyWithActiveRun(t, db)
	failDeletes(t, db, "escalation_policies")

	require.Error(t, s.DeletePolicy(ctx, policyID))

	run, err := s.SelectRun(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, escalation.RunStatusActive, run.Status)
}
