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

//go:build integration

package agentserver_test

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

// This file proves the reason the feature exists (US2, SC-001): the server is
// replaceable. Every test here destroys the whole server process state — the
// gRPC server, the stores, the database handle — and brings a new one up on
// the same external database. Nothing is done to the agents.

// failoverDSN creates an empty database on the configured test server, or
// skips. The database outlives each simulated instance, which is the point.
func failoverDSN(t *testing.T) string {
	t.Helper()
	adminDSN := storetest.AdminDSN(t)

	name := "t_" + strings.ReplaceAll(uid.New(), "-", "")
	admin, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err)
	_, err = admin.ExecContext(context.Background(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		_ = admin.Close()
	})

	u, err := store.ParseDSN(adminDSN)
	require.NoError(t, err)
	u.Path = "/" + name
	return u.String()
}

// instance is one simulated server process: its own database handle, its own
// stores, its own gRPC server. Stopping it leaves nothing behind but the
// external database.
type instance struct {
	db         *store.DB
	agentStore *store.AgentStore
	addr       string
	clientTLS  *tls.Config
	stop       func()
}

func startInstance(t *testing.T, dsn string) *instance {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	db, err := store.OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err, "open instance database")
	require.NoError(t, store.Migrate(ctx, db, logger))

	writerCtx, cancelWriter := context.WithCancel(context.Background())
	db.StartWriter(writerCtx)

	agentStore := store.NewAgentStore(db)
	addr, clientTLS := startTestServer(t, agentserver.Deps{
		AgentStore:  agentStore,
		Broadcaster: noopBroadcaster{},
		Logger:      logger,
	})

	return &instance{
		db:         db,
		agentStore: agentStore,
		addr:       addr,
		clientTLS:  clientTLS,
		stop: func() {
			cancelWriter()
			_ = db.Close()
		},
	}
}

func seedToken(t *testing.T, s *store.AgentStore, cleartext string, expiresAt time.Time) {
	t.Helper()
	require.NoError(t, s.InsertToken(context.Background(), &agent.EnrollmentToken{
		TokenID:     agent.HashToken(cleartext)[:16],
		TokenHash:   agent.HashToken(cleartext),
		TokenPrefix: agent.TokenPrefix(cleartext),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   expiresAt.UTC(),
	}))
}

func registerRequest(agentID, token, hostname string) *agentpb.RegisterRequest {
	return &agentpb.RegisterRequest{
		AgentId:         agentID,
		EnrollmentToken: token,
		Hostname:        hostname,
		Label:           hostname,
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       make([]byte, 32),
	}
}

// TestFailover_AgentsSurviveServerLoss is SC-001: the machine carrying the
// instance is lost, a new one comes up on the same database, and every agent
// is recognised as the same host with the same id. No re-enrolment, no token
// re-issued, no action on any monitored machine.
func TestFailover_AgentsSurviveServerLoss(t *testing.T) {
	withMultiHostEdition(t)
	dsn := failoverDSN(t)
	ctx := context.Background()

	// --- First instance: enrol a fleet of three. ---
	first := startInstance(t, dsn)

	type enrolled struct {
		id       string
		hostname string
	}
	fleet := make([]enrolled, 0, 3)
	firstClient := dialGRPC(t, first.addr, first.clientTLS)
	for i := range 3 {
		token := fmt.Sprintf("mnt_enr_failover%09d", i)
		seedToken(t, first.agentStore, token, time.Now().Add(time.Hour))

		agentID := uid.New()
		hostname := fmt.Sprintf("host-%d", i)
		_, err := firstClient.RegisterAgent(ctx, registerRequest(agentID, token, hostname))
		require.NoError(t, err, "enrolment %d", i)
		fleet = append(fleet, enrolled{id: agentID, hostname: hostname})
	}

	before, err := first.agentStore.List(ctx, "all")
	require.NoError(t, err)
	require.Len(t, before, len(fleet), "the fleet is enrolled")

	// --- The machine is lost: the whole process state goes. ---
	first.stop()

	// --- A replacement comes up elsewhere, same connection string. ---
	second := startInstance(t, dsn)

	after, err := second.agentStore.List(ctx, "all")
	require.NoError(t, err)
	assert.Len(t, after, len(fleet), "no agent lost, none duplicated (FR-008)")

	byID := map[string]bool{}
	for _, a := range after {
		byID[a.AgentID] = true
	}
	for _, want := range fleet {
		require.True(t, byID[want.id], "agent %s must be recognised as the same host", want.hostname)

		got, err := second.agentStore.Get(ctx, want.id)
		require.NoError(t, err, "the replacement knows %s without re-enrolment (FR-007)", want.hostname)
		assert.Equal(t, want.hostname, got.Hostname)
		assert.Equal(t, "active", got.Status)
		assert.Len(t, got.PublicKey, 32, "the identity key survived (FR-006)")
	}
}

// TestFailover_EnrolmentRulesUnchanged is US2 scenarios 3 and 4: after the
// switch, a token is accepted or refused by the same rules as before, and a
// revoked agent stays revoked. Never because the instance changed (FR-009).
func TestFailover_EnrolmentRulesUnchanged(t *testing.T) {
	withMultiHostEdition(t)
	dsn := failoverDSN(t)
	ctx := context.Background()

	first := startInstance(t, dsn)

	const (
		unusedToken  = "mnt_enr_failoverunused01"
		expiredToken = "mnt_enr_failoverexpired1"
		usedToken    = "mnt_enr_failoverused0001"
	)
	seedToken(t, first.agentStore, unusedToken, time.Now().Add(time.Hour))
	seedToken(t, first.agentStore, expiredToken, time.Now().Add(-time.Hour))
	seedToken(t, first.agentStore, usedToken, time.Now().Add(time.Hour))

	firstClient := dialGRPC(t, first.addr, first.clientTLS)

	// One agent enrols and is then revoked.
	revokedID := uid.New()
	_, err := firstClient.RegisterAgent(ctx, registerRequest(revokedID, usedToken, "host-revoked"))
	require.NoError(t, err)
	require.NoError(t, first.agentStore.Revoke(ctx, revokedID, "test"))

	first.stop()
	second := startInstance(t, dsn)
	secondClient := dialGRPC(t, second.addr, second.clientTLS)

	t.Run("a valid unused token is still accepted", func(t *testing.T) {
		_, err := secondClient.RegisterAgent(ctx, registerRequest(uid.New(), unusedToken, "host-late"))
		require.NoError(t, err, "the same rules apply, and the instance change is not one of them")
	})

	t.Run("an expired token is still refused", func(t *testing.T) {
		_, err := secondClient.RegisterAgent(ctx, registerRequest(uid.New(), expiredToken, "host-expired"))
		require.Error(t, err)
		st, ok := grpcstatus.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.FailedPrecondition, st.Code())
	})

	t.Run("a consumed token is still refused", func(t *testing.T) {
		_, err := secondClient.RegisterAgent(ctx, registerRequest(uid.New(), usedToken, "host-replay"))
		require.Error(t, err)
		st, ok := grpcstatus.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.FailedPrecondition, st.Code())
	})

	t.Run("a revoked agent stays revoked", func(t *testing.T) {
		got, err := second.agentStore.Get(ctx, revokedID)
		require.NoError(t, err)
		assert.Equal(t, "revoked", got.Status, "revocation survives the switch (FR-009)")
	})
}

// TestConcurrentInstances_NoDuplicateAgents is FR-011: the cluster manager is
// supposed to prevent two instances writing at once, but a partition or a
// human error will produce it one day. It must stay correct — no corruption,
// no duplicated agent, no silently overwritten data — and each instance must
// be able to see the other (FR-012).
func TestConcurrentInstances_NoDuplicateAgents(t *testing.T) {
	withMultiHostEdition(t)
	dsn := failoverDSN(t)
	ctx := context.Background()

	a := startInstance(t, dsn)
	defer a.stop()
	b := startInstance(t, dsn)
	defer b.stop()

	// Both instances take inventory reports for the same hosts at the same
	// time. Ids are deterministic and upserts update, so the two converge on
	// the same rows instead of duplicating them.
	const hosts = 6
	ids := make([]string, hosts)
	for i := range ids {
		ids[i] = uid.New()
	}

	var wg sync.WaitGroup
	for _, inst := range []*instance{a, b} {
		for i, id := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = inst.agentStore.Insert(ctx, &agent.Agent{
					AgentID:         id,
					PublicKey:       make([]byte, 32),
					Hostname:        fmt.Sprintf("shared-host-%d", i),
					Label:           fmt.Sprintf("shared-host-%d", i),
					OSArch:          "linux/amd64",
					AgentVersion:    "1.0.0",
					DetectedRuntime: "docker",
					Status:          "active",
					CreatedAt:       time.Now().UTC(),
				})
			}()
		}
	}
	wg.Wait()

	seen, err := a.agentStore.List(ctx, "all")
	require.NoError(t, err)
	assert.Len(t, seen, hosts, "concurrent writers converge, they do not duplicate (FR-011)")

	for _, id := range ids {
		got, err := a.agentStore.Get(ctx, id)
		require.NoError(t, err, "every host is readable and intact")
		assert.Equal(t, "active", got.Status)
	}

	// Each instance can see that another one works on this database.
	instStore := store.NewInstanceStore(a.db)
	selfA := store.Instance{
		ID: uid.New(), Hostname: "node-a", Version: "test",
		StartedAt: time.Now(), LastSeenAt: time.Now(),
	}
	selfB := store.Instance{
		ID: uid.New(), Hostname: "node-b", Version: "test",
		StartedAt: time.Now(), LastSeenAt: time.Now(),
	}
	require.NoError(t, instStore.Register(ctx, selfA))
	require.NoError(t, store.NewInstanceStore(b.db).Register(ctx, selfB))

	peers, err := instStore.Peers(ctx, selfA.ID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, peers, 1, "the second instance is visible, not silently tolerated (FR-012)")
	assert.Equal(t, "node-b", peers[0].Hostname)
}
