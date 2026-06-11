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

package agentserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

type noopBroadcasterUnit struct{}

func (noopBroadcasterUnit) BroadcastEvent(string, any) {}

// newHostLimitTestImpl builds an ingestImpl backed by a real temp SQLite store,
// without standing up the gRPC server — RegisterAgent is called in-process.
func newHostLimitTestImpl(t *testing.T) (*ingestImpl, *sqlite.AgentStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	require.NoError(t, sqlite.Migrate(db.ReadDB(), logger))
	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)
	t.Cleanup(func() { cancel(); _ = db.Close() })

	store := sqlite.NewAgentStore(db)
	impl := &ingestImpl{deps: Deps{AgentStore: store, Broadcaster: noopBroadcasterUnit{}, Logger: logger}}
	return impl, store
}

func insertActiveAgents(t *testing.T, store *sqlite.AgentStore, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		require.NoError(t, store.Insert(ctx, &agent.Agent{
			AgentID:         fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1),
			Hostname:        fmt.Sprintf("host-%d", i+1),
			Label:           fmt.Sprintf("host-%d", i+1),
			OSArch:          "linux/amd64",
			AgentVersion:    "1.0.0",
			DetectedRuntime: "docker",
			Status:          "active",
			CreatedAt:       time.Now().UTC(),
		}))
	}
}

func insertToken(t *testing.T, store *sqlite.AgentStore, id, tok string) string {
	t.Helper()
	require.NoError(t, store.InsertToken(context.Background(), &agent.EnrollmentToken{
		TokenID:   id,
		Token:     tok,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))
	return tok
}

func registerReq(agentID, token, hostname string) *agentpb.RegisterRequest {
	return &agentpb.RegisterRequest{
		AgentId:         agentID,
		EnrollmentToken: token,
		Hostname:        hostname,
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       make([]byte, 32),
	}
}

// At the Pro cap, a further enrollment is rejected with ResourceExhausted and
// the one-time token is left unconsumed (the check runs before ConsumeAtomic).
func TestRegisterAgent_HostLimit_RejectedAtCap(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	limit := extension.AgentHostLimit()
	require.Greater(t, limit, 0)
	insertActiveAgents(t, store, limit) // fill exactly to the cap

	tok := insertToken(t, store, "tok-overcap", "mnt_enr_overcaptoken0001")
	_, err := impl.RegisterAgent(ctx, registerReq(
		"11111111-1111-1111-1111-111111111111", tok, "over-cap"))
	require.Error(t, err)
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())

	gotTok, err := store.GetByToken(ctx, tok)
	require.NoError(t, err)
	assert.Nil(t, gotTok.ConsumedAt, "rejected enrollment must not consume the token")
}

// Under the cap, enrollment succeeds and the agent is persisted as active.
func TestRegisterAgent_HostLimit_UnderCapSucceeds(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	tok := insertToken(t, store, "tok-ok", "mnt_enr_undercaptoken001")
	const id = "22222222-2222-2222-2222-222222222222"
	_, err := impl.RegisterAgent(ctx, registerReq(id, tok, "under-cap"))
	require.NoError(t, err)

	got, err := store.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
}

// Community cannot enroll any agent (cap 0), even via a valid token.
func TestRegisterAgent_HostLimit_CommunityBlocked(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	tok := insertToken(t, store, "tok-ce", "mnt_enr_communitytoken01")
	_, err := impl.RegisterAgent(ctx, registerReq(
		"33333333-3333-3333-3333-333333333333", tok, "ce-host"))
	require.Error(t, err)
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}
