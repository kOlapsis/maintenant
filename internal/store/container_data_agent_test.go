// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

const homonym = "shop/Deployment/api"

func seedHomonyms(t *testing.T, db *DB) (staging, prod string) {
	t.Helper()
	cs := NewContainerStore(db)
	seedAgent(t, db, "agent-staging")
	seedAgent(t, db, "agent-prod")
	staging = seedHostedContainer(t, cs, "agent-staging", homonym, "api", "shop/api:1.0")
	prod = seedHostedContainer(t, cs, "agent-prod", homonym, "api", "shop/api:1.0")
	return staging, prod
}

var perAgentTables = []string{"image_updates", "container_cves", "version_pins", "risk_score_history",
	"cve_evaluations", "risk_acknowledgments", "digest_baselines"}

func seedAgentRows(t *testing.T, db *DB, agentID string) {
	t.Helper()
	now := time.Now().Unix()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO image_updates (id, agent_id, container_id, container_name, image, current_tag, current_digest,
			registry, latest_tag, status, detected_at) VALUES (?, ?, ?, 'api', 'shop/api', '1.0', '', 'ghcr.io', '1.1', 'available', ?)`,
			[]any{uid.New(), agentID, homonym, now}},
		{`INSERT INTO container_cves (id, agent_id, container_id, cve_id, severity, first_detected_at)
			VALUES (?, ?, ?, 'CVE-2026-0001', 'high', ?)`, []any{uid.New(), agentID, homonym, now}},
		{`INSERT INTO version_pins (id, agent_id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at)
			VALUES (?, ?, ?, 'shop/api', '1.0', 'sha256:aaa', 'frozen', ?)`, []any{uid.New(), agentID, homonym, now}},
		{`INSERT INTO risk_score_history (id, agent_id, container_id, score, factors_json, recorded_at)
			VALUES (?, ?, ?, 64, '{}', ?)`, []any{uid.New(), agentID, homonym, now}},
		{`INSERT INTO cve_evaluations (agent_id, container_id, status, evaluated_at) VALUES (?, ?, 'evaluated', ?)`,
			[]any{agentID, homonym, now}},
		{`INSERT INTO risk_acknowledgments (id, agent_id, container_external_id, finding_type, finding_key, acknowledged_at)
			VALUES (?, ?, ?, 'cve', 'CVE-2026-0001', ?)`, []any{uid.New(), agentID, homonym, now}},
		{`INSERT INTO digest_baselines (agent_id, container_id, image, tag, remote_digest, checked_at)
			VALUES (?, ?, 'shop/api:1.0', '1.0', 'sha256:bbb', ?)`, []any{agentID, homonym, now}},
	} {
		_, err := db.ReadDB().ExecContext(context.Background(), db.dialect.Rebind(q.sql), q.args...)
		require.NoError(t, err, q.sql)
	}
}

func agentRowCount(t *testing.T, db *DB, table, agentID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.ReadDB().QueryRowContext(context.Background(), db.dialect.Rebind(
		`SELECT COUNT(*) FROM `+table+` WHERE agent_id = ?`), agentID).Scan(&n))
	return n
}

// Deleting or purging a workload on one agent leaves every row of its homonym on another agent in place.
func TestContainerData_PurgeLeavesTheHomonymOfAnotherAgent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		purge func(t *testing.T, cs *ContainerStore, stagingUID string)
	}{
		{"delete", func(t *testing.T, cs *ContainerStore, stagingUID string) {
			require.NoError(t, cs.DeleteContainerByID(context.Background(), stagingUID))
		}},
		{"archive_purge", func(t *testing.T, cs *ContainerStore, stagingUID string) {
			ctx := context.Background()
			require.NoError(t, cs.ArchiveContainer(ctx, stagingUID, time.Now().Add(-48*time.Hour)))
			_, err := cs.DeleteArchivedContainersBefore(ctx, time.Now().Add(-time.Hour))
			require.NoError(t, err)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			stagingUID, _ := seedHomonyms(t, db)
			seedAgentRows(t, db, "agent-staging")
			seedAgentRows(t, db, "agent-prod")

			tc.purge(t, NewContainerStore(db), stagingUID)

			for _, table := range perAgentTables {
				assert.Equal(t, 1, agentRowCount(t, db, table, "agent-prod"), "%s of the prod workload", table)
			}
			for _, table := range []string{"image_updates", "container_cves", "version_pins", "risk_score_history"} {
				assert.Zero(t, agentRowCount(t, db, table, "agent-staging"), "%s of the purged workload", table)
			}
		})
	}
}

// The digest baseline of an archived workload goes even while its homonym runs on another agent.
func TestCleanupExpired_DigestBaselineFollowsItsAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	stagingUID, _ := seedHomonyms(t, db)
	seedAgentRows(t, db, "agent-staging")
	seedAgentRows(t, db, "agent-prod")
	require.NoError(t, NewContainerStore(db).ArchiveContainer(ctx, stagingUID, time.Now()))

	_, err := NewUpdateStore(db).CleanupExpired(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)

	assert.Zero(t, agentRowCount(t, db, "digest_baselines", "agent-staging"))
	assert.Equal(t, 1, agentRowCount(t, db, "digest_baselines", "agent-prod"))
}

// Pinning a workload on one agent leaves its homonym on another agent unpinned, and each keeps its own pin.
func TestVersionPin_IsPerAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedHomonyms(t, db)
	us := NewUpdateStore(db)

	pin := func(agentID, reason string) {
		t.Helper()
		_, err := us.InsertVersionPin(ctx, &update.VersionPin{AgentID: agentID, ContainerID: homonym,
			Image: "shop/api", PinnedTag: "1.0", PinnedDigest: "sha256:aaa", Reason: reason, PinnedAt: time.Now()})
		require.NoError(t, err)
	}
	reason := func(agentID string) string {
		t.Helper()
		p, err := us.GetVersionPin(ctx, agentID, homonym)
		require.NoError(t, err)
		if p == nil {
			return ""
		}
		assert.Equal(t, agentID, p.AgentID)
		return p.Reason
	}

	pin("agent-staging", "staging freeze")
	assert.Equal(t, "staging freeze", reason("agent-staging"))
	assert.Empty(t, reason("agent-prod"), "pinning staging does not pin prod")

	pin("agent-prod", "prod freeze")
	assert.Equal(t, "staging freeze", reason("agent-staging"))
	assert.Equal(t, "prod freeze", reason("agent-prod"))

	require.NoError(t, us.DeleteVersionPin(ctx, "agent-staging", homonym))
	assert.Empty(t, reason("agent-staging"))
	assert.Equal(t, "prod freeze", reason("agent-prod"), "unpinning staging keeps the prod pin")
}

// What the enrichment records for a workload on one agent is not read for its homonym on another.
func TestContainerData_ReadsArePerAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedHomonyms(t, db)
	us := NewUpdateStore(db)
	acks := NewAcknowledgmentStore(db)
	now := time.Now()

	require.NoError(t, us.UpsertContainerCVE(ctx, &update.ContainerCVE{AgentID: "agent-staging", ContainerID: homonym,
		CVEID: "CVE-2026-0001", Severity: update.CVESeverityHigh, FirstDetectedAt: now}))
	require.NoError(t, us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{AgentID: "agent-staging", ContainerID: homonym,
		Status: update.CVEEvaluated, EvaluatedAt: now}))
	require.NoError(t, us.UpsertDigestBaseline(ctx, &update.DigestBaseline{AgentID: "agent-staging", ContainerID: homonym,
		Image: "shop/api:1.0", Tag: "1.0", RemoteDigest: "sha256:bbb", CheckedAt: now}))
	_, err := us.InsertRiskScoreRecord(ctx, &update.RiskScoreRecord{AgentID: "agent-staging", ContainerID: homonym,
		Score: 64, FactorsJSON: "{}", RecordedAt: now})
	require.NoError(t, err)
	_, err = acks.InsertAcknowledgment(ctx, &security.RiskAcknowledgment{AgentID: "agent-staging",
		ContainerExternalID: homonym, FindingType: "cve", FindingKey: "CVE-2026-0001", AcknowledgedAt: now})
	require.NoError(t, err)

	for agentID, want := range map[string]int{"agent-staging": 1, "agent-prod": 0} {
		cves, err := us.ListContainerCVEs(ctx, agentID, homonym)
		require.NoError(t, err)
		assert.Len(t, cves, want, "CVEs of %s", agentID)
		active, err := us.ListAllActiveCVEs(ctx, update.ListCVEsOpts{AgentID: agentID, ContainerID: homonym})
		require.NoError(t, err)
		assert.Len(t, active, want, "active CVEs of %s", agentID)

		eval, err := us.GetCVEEvaluation(ctx, agentID, homonym)
		require.NoError(t, err)
		assert.Equal(t, want == 1, eval != nil, "CVE evaluation of %s", agentID)

		baseline, err := us.GetDigestBaseline(ctx, agentID, homonym)
		require.NoError(t, err)
		assert.Equal(t, want == 1, baseline != nil, "digest baseline of %s", agentID)

		history, err := us.ListRiskScoreHistory(ctx, agentID, homonym, now.Add(-time.Hour), now.Add(time.Hour))
		require.NoError(t, err)
		assert.Len(t, history, want, "risk history of %s", agentID)

		acked, err := acks.IsAcknowledged(ctx, agentID, homonym, "cve", "CVE-2026-0001")
		require.NoError(t, err)
		assert.Equal(t, want == 1, acked, "acknowledgement of %s", agentID)
		listed, err := acks.ListAcknowledgments(ctx, agentID, homonym)
		require.NoError(t, err)
		assert.Len(t, listed, want, "listed acknowledgements of %s", agentID)
	}

	_, err = acks.InsertAcknowledgment(ctx, &security.RiskAcknowledgment{AgentID: "agent-prod",
		ContainerExternalID: homonym, FindingType: "cve", FindingKey: "CVE-2026-0001", AcknowledgedAt: now})
	require.NoError(t, err, "the prod homonym can acknowledge the same finding on its own")
}

// The same key written for two agents makes two rows, and rewriting one agent's row leaves the other's as it was.
func TestContainerData_SameKeyOnTwoAgents(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedHomonyms(t, db)
	us := NewUpdateStore(db)
	now := time.Now().Truncate(time.Second)

	write := func(agentID, value string, recordedAt time.Time) {
		t.Helper()
		require.NoError(t, us.UpsertContainerCVE(ctx, &update.ContainerCVE{AgentID: agentID, ContainerID: homonym,
			CVEID: "CVE-2026-0001", Severity: update.CVESeverityHigh, Summary: value, FirstDetectedAt: now}))
		require.NoError(t, us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{AgentID: agentID, ContainerID: homonym,
			Status: update.CVEEvaluationError, Error: value, EvaluatedAt: now}))
		require.NoError(t, us.UpsertDigestBaseline(ctx, &update.DigestBaseline{AgentID: agentID, ContainerID: homonym,
			Image: "shop/api:1.0", Tag: "1.0", RemoteDigest: value, CheckedAt: now}))
		_, err := us.InsertRiskScoreRecord(ctx, &update.RiskScoreRecord{AgentID: agentID, ContainerID: homonym,
			Score: 64, FactorsJSON: value, RecordedAt: recordedAt})
		require.NoError(t, err)
		_, err = us.InsertVersionPin(ctx, &update.VersionPin{AgentID: agentID, ContainerID: homonym,
			Image: "shop/api", PinnedTag: "1.0", PinnedDigest: "sha256:aaa", Reason: value, PinnedAt: now})
		require.NoError(t, err)
	}
	read := func(agentID string) []string {
		t.Helper()
		cves, err := us.ListContainerCVEs(ctx, agentID, homonym)
		require.NoError(t, err)
		require.Len(t, cves, 1, "CVEs of %s", agentID)
		eval, err := us.GetCVEEvaluation(ctx, agentID, homonym)
		require.NoError(t, err)
		require.NotNil(t, eval)
		baseline, err := us.GetDigestBaseline(ctx, agentID, homonym)
		require.NoError(t, err)
		require.NotNil(t, baseline)
		pin, err := us.GetVersionPin(ctx, agentID, homonym)
		require.NoError(t, err)
		require.NotNil(t, pin)
		history, err := us.ListRiskScoreHistory(ctx, agentID, homonym, now.Add(-time.Hour), now.Add(time.Hour))
		require.NoError(t, err)
		values := []string{cves[0].Summary, eval.Error, baseline.RemoteDigest, pin.Reason}
		for _, h := range history {
			values = append(values, h.FactorsJSON)
		}
		return values
	}

	write("agent-staging", "staging", now)
	write("agent-prod", "prod", now)
	assert.Equal(t, []string{"staging", "staging", "staging", "staging", "staging"}, read("agent-staging"))
	assert.Equal(t, []string{"prod", "prod", "prod", "prod", "prod"}, read("agent-prod"))

	write("agent-staging", "staging-2", now.Add(time.Second))
	assert.Equal(t, []string{"staging-2", "staging-2", "staging-2", "staging-2", "staging", "staging-2"}, read("agent-staging"))
	assert.Equal(t, []string{"prod", "prod", "prod", "prod", "prod"}, read("agent-prod"), "rewriting staging keeps prod")
}

// An external id names the local runtime when it runs it or nothing does, else its only agent; two remote agents need naming.
func TestContainerAgent_LocalFirstThenTheOnlyRemote(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedHomonyms(t, db)
	cs := NewContainerStore(db)
	seedHostedContainer(t, cs, "agent-prod", "ctr-only-prod", "web", "nginx:1.27")
	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-shared", "web", "nginx:1.27")
	seedHostedContainer(t, cs, "agent-prod", "ctr-shared", "web", "nginx:1.27")
	us := NewUpdateStore(db)

	for externalID, want := range map[string]string{
		"ctr-only-prod": "agent-prod",
		"ctr-shared":    uid.LocalAgent,
		"ctr-unknown":   uid.LocalAgent,
	} {
		got, err := us.ContainerAgent(ctx, externalID)
		require.NoError(t, err)
		assert.Equal(t, want, got, externalID)
	}

	_, err := us.ContainerAgent(ctx, homonym)
	assert.ErrorIs(t, err, update.ErrAmbiguousAgent, "staging and prod both run it")
}
