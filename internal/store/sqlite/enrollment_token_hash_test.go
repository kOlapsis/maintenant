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

package sqlite

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
)

// downgradeEnrollmentTokens restores the cleartext-token table, standing in for
// a database converted before the token was hashed. Two rows: one still usable,
// one already consumed (kept for audit).
const downgradeEnrollmentTokens = `
DROP TABLE enrollment_tokens;
CREATE TABLE enrollment_tokens (
    id            TEXT PRIMARY KEY NOT NULL,
    token         TEXT NOT NULL UNIQUE,
    created_at    BIGINT NOT NULL DEFAULT 0,
    expires_at    BIGINT NOT NULL,
    consumed_at   BIGINT,
    consumed_by_agent_id TEXT
);
CREATE INDEX idx_enrollment_tokens_expires_at ON enrollment_tokens(expires_at);
INSERT INTO enrollment_tokens (id, token, created_at, expires_at, consumed_at, consumed_by_agent_id)
VALUES ('pending000000001', 'mnt_enr_pendinglegacytoken', 1700000000, 4000000000, NULL, NULL),
       ('consumed00000001', 'mnt_enr_consumedlegacytok', 1700000000, 4000000000, 1700000100, 'some-agent');
`

const legacyPendingToken = "mnt_enr_pendinglegacytoken"

// A token handed out before the upgrade must still enroll afterwards. The
// rebuild derives the hash from the stored cleartext, so nobody has to reissue
// tokens — and once it has run, the cleartext is gone for good.
func TestRebuildEnrollmentTokensForHashing(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := rw.ExecContext(ctx, downgradeEnrollmentTokens)
	require.NoError(t, err)

	require.NoError(t, rebuildEnrollmentTokensForHashing(ctx, rw, logger))

	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='enrollment_tokens'`)
	require.Contains(t, ddl, enrollmentTokenHashColumn)
	require.NotContains(t, ddl, "token         TEXT NOT NULL UNIQUE",
		"the cleartext column must be gone, not merely unused")

	// Both rows survived, hashed, with their audit fields intact.
	var count int
	require.NoError(t, rw.QueryRowContext(ctx, `SELECT COUNT(*) FROM enrollment_tokens`).Scan(&count))
	require.Equal(t, 2, count)

	assert.Equal(t, agent.HashToken(legacyPendingToken),
		scanString(t, rw, `SELECT token_hash FROM enrollment_tokens WHERE id='pending000000001'`))
	assert.Equal(t, agent.TokenPrefix(legacyPendingToken),
		scanString(t, rw, `SELECT token_prefix FROM enrollment_tokens WHERE id='pending000000001'`))
	assert.Equal(t, "some-agent",
		scanString(t, rw, `SELECT consumed_by_agent_id FROM enrollment_tokens WHERE id='consumed00000001'`))

	// The pending token is still redeemable through the normal store path.
	store := NewAgentStore(db)
	got, err := store.GetByToken(ctx, legacyPendingToken)
	require.NoError(t, err, "a token issued before the upgrade must still resolve")
	assert.Equal(t, "pending000000001", got.TokenID)
	assert.Nil(t, got.ConsumedAt)

	// The index was recreated after the DROP TABLE.
	var indexes int
	require.NoError(t, rw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='enrollment_tokens' AND name LIKE 'idx_%'`).Scan(&indexes))
	assert.Equal(t, 1, indexes)

	// Second run is a no-op.
	require.NoError(t, rebuildEnrollmentTokensForHashing(ctx, rw, logger))
	assert.Equal(t, ddl, scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='enrollment_tokens'`))
}

// The rebuild drops the live enrollment_tokens table. That is only acceptable
// because the whole sequence is one transaction: if any statement fails, the
// original table and every row in it must still be there.
//
// The failure is injected by squatting the scratch table name, which makes the
// first statement (CREATE TABLE enrollment_tokens_new) fail before the copy.
func TestRebuildEnrollmentTokensForHashing_RollsBackOnFailure(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := rw.ExecContext(ctx, downgradeEnrollmentTokens)
	require.NoError(t, err)
	_, err = rw.ExecContext(ctx, `CREATE TABLE enrollment_tokens_new (squatted TEXT)`)
	require.NoError(t, err)

	require.Error(t, rebuildEnrollmentTokensForHashing(ctx, rw, logger),
		"the rebuild must report the failure rather than swallow it")

	// The original table is untouched: same DDL, same rows, cleartext intact.
	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='enrollment_tokens'`)
	assert.Contains(t, ddl, "token         TEXT NOT NULL UNIQUE", "the old table must survive a failed rebuild")

	var count int
	require.NoError(t, rw.QueryRowContext(ctx, `SELECT COUNT(*) FROM enrollment_tokens`).Scan(&count))
	assert.Equal(t, 2, count, "no row may be lost when the rebuild aborts")
	assert.Equal(t, legacyPendingToken,
		scanString(t, rw, `SELECT token FROM enrollment_tokens WHERE id='pending000000001'`))

	// Clearing the obstruction lets the next boot complete the rebuild.
	_, err = rw.ExecContext(ctx, `DROP TABLE enrollment_tokens_new`)
	require.NoError(t, err)
	require.NoError(t, rebuildEnrollmentTokensForHashing(ctx, rw, logger))
	assert.Equal(t, agent.HashToken(legacyPendingToken),
		scanString(t, rw, `SELECT token_hash FROM enrollment_tokens WHERE id='pending000000001'`))
}

// After the rebuild the live table holds no cleartext. This asserts the stored
// columns only — it says nothing about pages freed inside the file, which
// secure_delete addresses but which no test here proves.
func TestRebuildEnrollmentTokensForHashing_LeavesNoCleartext(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := rw.ExecContext(ctx, downgradeEnrollmentTokens)
	require.NoError(t, err)
	require.NoError(t, rebuildEnrollmentTokensForHashing(ctx, rw, logger))

	rows, err := rw.QueryContext(ctx,
		`SELECT id, token_hash, token_prefix, consumed_by_agent_id FROM enrollment_tokens`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, hash, prefix string
		var consumedBy *string
		require.NoError(t, rows.Scan(&id, &hash, &prefix, &consumedBy))
		assert.NotContains(t, hash, "mnt_enr_", "the hash must not carry the token")
		// The prefix is the deliberate exception: 14 chars of a 256-bit secret,
		// kept so the UI can still name a token it can no longer read.
		assert.LessOrEqual(t, len(prefix), agent.TokenPrefixLen)
	}
	require.NoError(t, rows.Err())
}

// A fresh install must come out of uuid_schema.sql already hashed, never
// through the rebuild.
func TestFreshSchema_EnrollmentTokensAreHashed(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()

	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='enrollment_tokens'`)
	assert.Contains(t, ddl, enrollmentTokenHashColumn)
	assert.Contains(t, ddl, "token_prefix")
	assert.NotContains(t, ddl, "token         TEXT NOT NULL UNIQUE")
}

// The rebuild's CREATE TABLE must embed the exact column string the idempotency
// guard looks for, or fresh installs would be rebuilt on every boot.
func TestEnrollmentTokenHashColumn_MatchesSchema(t *testing.T) {
	schema, err := os.ReadFile("uuid_schema.sql")
	require.NoError(t, err)
	require.True(t, strings.Contains(string(schema), enrollmentTokenHashColumn),
		"uuid_schema.sql must contain %q verbatim — the rebuild guard depends on it", enrollmentTokenHashColumn)
}

// End-to-end through the store: what goes in as cleartext is queryable by
// cleartext, but only ever stored as a hash.
func TestAgentStore_TokenRoundTripIsHashed(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	cleartext, hash, id, prefix, err := agent.NewToken()
	require.NoError(t, err)
	require.NoError(t, store.InsertToken(ctx, &agent.EnrollmentToken{
		TokenID:     id,
		TokenHash:   hash,
		TokenPrefix: prefix,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}))

	got, err := store.GetByToken(ctx, cleartext)
	require.NoError(t, err)
	assert.Equal(t, hash, got.TokenHash)
	assert.Equal(t, prefix, got.TokenPrefix)
	assert.Equal(t, prefix+"...***", got.Masked())

	// A near-miss must not resolve: the lookup matches the full hash, not the
	// truncated id, so guessing the id buys nothing.
	_, err = store.GetByToken(ctx, cleartext+"x")
	assert.ErrorIs(t, err, agent.ErrTokenNotFound)

	stored := scanString(t, db.ReadDB(), `SELECT token_hash FROM enrollment_tokens`)
	assert.NotEqual(t, cleartext, stored)
	assert.Equal(t, id, agent.TokenIDFromHash(stored), "the id stays derivable from the hash")
}
