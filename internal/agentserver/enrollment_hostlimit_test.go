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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

type noopBroadcasterUnit struct{}

func (noopBroadcasterUnit) BroadcastEvent(string, any) {}

// newHostLimitTestImpl builds an ingestImpl backed by a real temp SQLite store,
// without standing up the gRPC server — RegisterAgent is called in-process.
func newHostLimitTestImpl(t *testing.T) (*ingestImpl, *store.AgentStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	store := store.NewAgentStore(db)
	impl := &ingestImpl{deps: Deps{AgentStore: store, Broadcaster: noopBroadcasterUnit{}, Logger: logger}}
	return impl, store
}

func insertActiveAgents(t *testing.T, store *store.AgentStore, n int) {
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

// insertToken stores tok the way the creation handler does — hash and display
// prefix only — and returns the cleartext for the caller to enroll with.
func insertToken(t *testing.T, store *store.AgentStore, id, tok string) string {
	t.Helper()
	require.NoError(t, store.InsertToken(context.Background(), &agent.EnrollmentToken{
		TokenID:     id,
		TokenHash:   agent.HashToken(tok),
		TokenPrefix: agent.TokenPrefix(tok),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
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

// At the Personal cap, a further enrollment is rejected with ResourceExhausted
// and the one-time token is left unconsumed (the check runs before the token is
// consumed). Personal is the edition that carries a cap now: Community enrolls
// nothing, Pro is unlimited.
func TestRegisterAgent_HostLimit_RejectedAtCap(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Personal }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	limit := extension.Limit(extension.ResourceAgentHosts)
	require.Equal(t, 20, limit)
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
	extension.CurrentEdition = func() extension.Edition { return extension.Personal }
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

// Concurrent enrollments racing on an empty Personal tier (cap 20) with
// distinct tokens must never exceed the cap: exactly 20 succeed, the rest are
// rejected with ResourceExhausted, and the persisted active count lands exactly
// on 20. This is the end-to-end regression test for the enroll-cap race, and it
// is what makes FR-026 hold.
func TestRegisterAgent_HostLimit_ConcurrentEnrollments(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Personal }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	limit := extension.Limit(extension.ResourceAgentHosts)
	require.Equal(t, 20, limit)
	const goroutines = 40
	for i := range goroutines {
		insertToken(t, store, fmt.Sprintf("tok-%d", i), fmt.Sprintf("mnt_enr_concurrent%07d", i))
	}

	var (
		ok    atomic.Int32
		atCap atomic.Int32
		other atomic.Int32
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_, err := impl.RegisterAgent(ctx, registerReq(
				fmt.Sprintf("aaaaaaaa-0000-0000-0000-%012d", i+1),
				fmt.Sprintf("mnt_enr_concurrent%07d", i),
				fmt.Sprintf("conc-host-%d", i)))
			switch {
			case err == nil:
				ok.Add(1)
			case grpcstatus.Code(err) == codes.ResourceExhausted:
				atCap.Add(1)
			default:
				other.Add(1)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(limit), ok.Load(), "exactly `limit` enrollments succeed")
	assert.Equal(t, int32(goroutines-limit), atCap.Load(), "the rest hit ResourceExhausted")
	assert.Equal(t, int32(0), other.Load(), "no unexpected errors")

	active, _, err := store.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, limit, active, "active agents never exceed the cap")
}

// A malformed public key is rejected before any store write: an ed25519 key
// must be exactly 32 bytes. The agent is never persisted and the token stays
// unconsumed. This is the enroll-side companion to the auth-path key-length
// guard that closes the unauthenticated-DoS vector.
func TestRegisterAgent_RejectsMalformedPublicKey(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	for name, key := range map[string][]byte{
		"nil":   nil,
		"empty": {},
		"short": make([]byte, 16),
		"long":  make([]byte, 64),
	} {
		t.Run(name, func(t *testing.T) {
			tok := insertToken(t, store, "tok-"+name, "mnt_enr_badkey"+fmt.Sprintf("%010s", name))
			const id = "44444444-4444-4444-4444-444444444444"
			req := registerReq(id, tok, "bad-key-host")
			req.PublicKey = key

			_, err := impl.RegisterAgent(ctx, req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))

			_, err = store.Get(ctx, id)
			assert.Error(t, err, "malformed enrollment must not persist an agent")

			gotTok, err := store.GetByToken(ctx, tok)
			require.NoError(t, err)
			assert.Nil(t, gotTok.ConsumedAt, "rejected enrollment must not consume the token")
		})
	}
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

// Community runs standalone: multi-host is a Personal capability, so the cap is
// zero and the very first enrollment is refused. The refusal is capacity
// (ResourceExhausted), never an authentication failure, and the one-time token
// survives so it can be used once the edition allows it.
func TestRegisterAgent_HostLimit_CommunityRefusesEveryEnrollment(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	require.Equal(t, 0, extension.Limit(extension.ResourceAgentHosts))

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	tok := insertToken(t, store, "tok-ce", "mnt_enr_communitytoken01")
	_, err := impl.RegisterAgent(ctx, registerReq(
		"33333333-3333-3333-3333-333333333333", tok, "community-host"))
	require.Error(t, err)

	st, ok := grpcstatus.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code(),
		"a capacity refusal must stay distinct from the token failures")

	gotTok, err := store.GetByToken(ctx, tok)
	require.NoError(t, err)
	assert.Nil(t, gotTok.ConsumedAt, "a refused enrollment must not consume the token")
}

// Pro has no cap: enrolling past what any lower edition allows must succeed.
func TestRegisterAgent_HostLimit_ProIsUnlimited(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	require.Equal(t, extension.Unlimited, extension.Limit(extension.ResourceAgentHosts))

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	// Well past the Personal cap of 20.
	insertActiveAgents(t, store, 25)

	tok := insertToken(t, store, "tok-pro", "mnt_enr_prounlimited0001")
	const id = "44444444-4444-4444-4444-444444444444"
	_, err := impl.RegisterAgent(ctx, registerReq(id, tok, "agent-26"))
	require.NoError(t, err, "Pro must not cap enrollment")

	got, err := store.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
}

// The machine running the application is an agent too, but it is never counted
// against the cap — only remotely enrolled hosts consume capacity (FR-025).
func TestRegisterAgent_HostLimit_LocalAgentIsNotCounted(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Personal }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	impl, store := newHostLimitTestImpl(t)
	ctx := context.Background()

	// The sentinel is seeded active by the schema, so it is always present.
	local, err := store.Get(ctx, uid.LocalAgent)
	require.NoError(t, err)
	require.NotNil(t, local, "the local sentinel agent must exist")
	require.Equal(t, "active", local.Status)

	limit := extension.Limit(extension.ResourceAgentHosts)
	insertActiveAgents(t, store, limit-1) // one slot left among remote hosts

	// With the local sentinel active on top of limit-1 remote hosts, a counting
	// bug that included it would see the cap as full and refuse here.
	tok := insertToken(t, store, "tok-local", "mnt_enr_localnotcounted1")
	const id = "55555555-5555-5555-5555-555555555555"
	_, err = impl.RegisterAgent(ctx, registerReq(id, tok, "last-slot"))
	require.NoError(t, err, "the local runtime must not consume a remote slot")

	// And the slot it filled was genuinely the last one.
	tok2 := insertToken(t, store, "tok-local-2", "mnt_enr_localnotcounted2")
	_, err = impl.RegisterAgent(ctx, registerReq(
		"66666666-6666-6666-6666-666666666666", tok2, "over-cap"))
	require.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err))
}
