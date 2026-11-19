// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

// The operator decisions a copy carries arrive under the agent they were taken for.
func TestCopy_OperatorDecisionsKeepTheirAgent(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)
	ctx := context.Background()

	var remote string
	require.NoError(t, src.ReadDB().QueryRowContext(ctx,
		`SELECT agent_id FROM containers WHERE external_id = 'ext-1'`).Scan(&remote))
	require.NotEqual(t, uid.LocalAgent, remote)
	now := time.Now().Unix()
	_, err := src.ReadDB().ExecContext(ctx, `INSERT INTO version_pins (id, agent_id, container_id, image, pinned_tag,
		pinned_digest, reason, pinned_at) VALUES (?, ?, 'ext-1', 'nginx', '1', 'sha256:abc', 'edge freeze', ?)`,
		uid.New(), remote, now)
	require.NoError(t, err)
	_, err = src.ReadDB().ExecContext(ctx, `INSERT INTO risk_acknowledgments (id, agent_id, container_external_id,
		finding_type, finding_key, reason, acknowledged_at) VALUES (?, ?, 'ext-1', 'cve', 'CVE-2026-1', 'edge only', ?)`,
		uid.New(), remote, now)
	require.NoError(t, err)

	_, err = Copy(ctx, src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.NoError(t, err)

	var reason string
	require.NoError(t, dst.QueryRow(`SELECT reason FROM version_pins WHERE agent_id = $1 AND container_id = 'ext-1'`,
		remote).Scan(&reason))
	assert.Equal(t, "edge freeze", reason)
	require.NoError(t, dst.QueryRow(`SELECT reason FROM risk_acknowledgments
		WHERE agent_id = $1 AND container_external_id = 'ext-1' AND finding_key = 'CVE-2026-1'`, remote).Scan(&reason))
	assert.Equal(t, "edge only", reason)
	require.NoError(t, dst.QueryRow(`SELECT reason FROM risk_acknowledgments
		WHERE agent_id = $1 AND container_external_id = 'ext-1' AND finding_key = 'CVE-2026-1'`, uid.LocalAgent).Scan(&reason))
	assert.Equal(t, "not exploitable", reason, "the local acknowledgement of the same finding travels apart")
}
