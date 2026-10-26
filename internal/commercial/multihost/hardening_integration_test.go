// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

//go:build integration

package multihost_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/commercial/multihost"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
)

// Acceptance tests of 046-agent-grpc-hardening over a real TCP gRPC server:
// US2 (C12) limits of the agent listener, and the agent binary pinning the
// certificate end to end (US1).

// dialFrom opens a fresh connection to addr from the given loopback address, so
// every call gets its own source port and the per-address quota is keyed on the IP.
func dialFrom(t *testing.T, addr string, tlsCfg *tls.Config, localIP string) agentpb.IngestClient {
	t.Helper()
	d := &net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(localIP)}}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithContextDialer(func(ctx context.Context, target string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp", target)
		}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return agentpb.NewIngestClient(conn)
}

func storeToken(t *testing.T, st *store.AgentStore, cleartext string) {
	t.Helper()
	require.NoError(t, st.InsertToken(context.Background(), &agent.EnrollmentToken{
		TokenID:     agent.TokenIDFromHash(agent.HashToken(cleartext)),
		TokenHash:   agent.HashToken(cleartext),
		TokenPrefix: agent.TokenPrefix(cleartext),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}))
}

func enrollRequest(agentID, token string, pub ed25519.PublicKey) *agentpb.RegisterRequest {
	if pub == nil {
		pub = make([]byte, 32)
	}
	return &agentpb.RegisterRequest{
		AgentId:         agentID,
		EnrollmentToken: token,
		Hostname:        "host",
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       []byte(pub),
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// authenticate enrolls agentID with a fresh keypair, opens Push and answers the challenge as authAgentID.
func authenticate(t *testing.T, ctx context.Context, client agentpb.IngestClient, st *store.AgentStore, agentID, authAgentID, token string) agentpb.Ingest_PushClient {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	storeToken(t, st, token)
	_, err = client.RegisterAgent(ctx, enrollRequest(agentID, token, pub))
	require.NoError(t, err, "enroll")

	stream, err := client.Push(ctx)
	require.NoError(t, err)
	srvMsg, err := stream.Recv()
	require.NoError(t, err)
	challenge, ok := srvMsg.GetPayload().(*agentpb.ServerMessage_Challenge)
	require.True(t, ok)
	now := time.Now().Unix()
	payload, err := buildSignPayload(challenge.Challenge.GetNonce(), authAgentID, now)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&agentpb.ClientMessage{Payload: &agentpb.ClientMessage_Auth{Auth: &agentpb.AuthResponse{
		AgentId: authAgentID, Timestamp: now, Signature: ed25519.Sign(priv, payload),
	}}}))
	return stream
}

// US2 scenario 1, FR-010, SC-003: a Push that never answers the challenge is closed
// with a deadline error between 30 and 35 seconds.
func TestIntegration_PushAuthTimesOutAfter30s(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	sessions := multihost.NewSessions(logger, noopBroadcaster{})
	addr, clientTLS := startTestServer(t, multihost.Deps{
		AgentStore: store.NewAgentStore(openIntegrationDB(t)), Broadcaster: noopBroadcaster{}, Sessions: sessions, Logger: logger,
	})
	client := dialGRPC(t, addr, clientTLS)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := client.Push(ctx)
	require.NoError(t, err)
	first, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, first.GetChallenge(), "the server opens with a challenge")

	start := time.Now()
	_, err = stream.Recv()
	elapsed := time.Since(start)
	require.Error(t, err, "a mute client must be closed")
	assert.Equal(t, codes.DeadlineExceeded, grpcstatus.Code(err), "got %v", err)
	assert.Contains(t, grpcstatus.Convert(err).Message(), "authentication timed out")
	assert.GreaterOrEqual(t, elapsed, 29500*time.Millisecond, "closed too early")
	assert.LessOrEqual(t, elapsed, 35*time.Second, "closed too late")
}

// FR-011 "durée de l'établissement d'une connexion": a TCP client that never starts the
// handshake is dropped by the server after 30 seconds.
func TestIntegration_ConnectionHandshakeTimesOut(t *testing.T) {
	logger := quietLogger()
	addr, _ := startTestServer(t, multihost.Deps{
		AgentStore: store.NewAgentStore(openIntegrationDB(t)), Broadcaster: noopBroadcaster{}, Logger: logger,
	})

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(50*time.Second)))

	start := time.Now()
	_, err = conn.Read(make([]byte, 1))
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.False(t, errors.Is(err, os.ErrDeadlineExceeded), "the server, not the client deadline, must end the connection")
	assert.GreaterOrEqual(t, elapsed, 25*time.Second)
	assert.LessOrEqual(t, elapsed, 36*time.Second, "a connection that never handshakes is dropped after 30 s")
}

// US2 scenario 2, FR-011: a message over 1 MiB is refused with ResourceExhausted; a smaller one is processed.
func TestIntegration_MessageOverOneMiBIsRefused(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	addr, clientTLS := startTestServer(t, multihost.Deps{
		AgentStore: store.NewAgentStore(openIntegrationDB(t)), Broadcaster: noopBroadcaster{}, Logger: logger,
	})
	client := dialGRPC(t, addr, clientTLS)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, size := range []int{1<<20 + 1024, 2 << 20} {
		req := enrollRequest("a1b2c3d4-0000-4000-8000-000000000b16", "mnt_enr_whatever", nil)
		req.PublicKey = make([]byte, size)
		_, err := client.RegisterAgent(ctx, req)
		require.Error(t, err, "%d bytes", size)
		assert.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err), "%d bytes: %v", size, err)
	}

	req := enrollRequest("a1b2c3d4-0000-4000-8000-000000000b17", "mnt_enr_whatever", nil)
	req.PublicKey = make([]byte, 512<<10)
	_, err := client.RegisterAgent(ctx, req)
	require.Error(t, err)
	assert.NotEqual(t, codes.ResourceExhausted, grpcstatus.Code(err), "512 KiB is under the limit and reaches the handler: %v", err)
}

// US2 scenario 3, FR-012: more than 10 RegisterAgent per minute from one address are
// refused; another address is not affected.
func TestIntegration_RegisterAgentIsRateLimitedPerAddress(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	st := store.NewAgentStore(openIntegrationDB(t))
	addr, clientTLS := startTestServer(t, multihost.Deps{AgentStore: st, Broadcaster: noopBroadcaster{}, Logger: logger})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 1; i <= 15; i++ {
		client := dialFrom(t, addr, clientTLS, "127.0.0.21")
		_, err := client.RegisterAgent(ctx, enrollRequest(fmt.Sprintf("a1b2c3d4-0000-4000-8000-0000000000%02d", i), "mnt_enr_unknown", nil))
		require.Error(t, err)
		if i <= 10 {
			assert.Equal(t, codes.NotFound, grpcstatus.Code(err), "call %d is still within the quota: %v", i, err)
		} else {
			assert.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err), "call %d is over the quota: %v", i, err)
			assert.Contains(t, grpcstatus.Convert(err).Message(), "too many connection attempts")
		}
	}

	const token = "mnt_enr_otheraddress01"
	storeToken(t, st, token)
	other := dialFrom(t, addr, clientTLS, "127.0.0.22")
	_, err := other.RegisterAgent(ctx, enrollRequest("a1b2c3d4-0000-4000-8000-000000000022", token, nil))
	require.NoError(t, err, "another address keeps its own quota")
	_, err = st.Get(ctx, "a1b2c3d4-0000-4000-8000-000000000022")
	require.NoError(t, err)
}

// US2 scenario 3, FR-012: opening Push counts against the same kind of quota before authentication.
func TestIntegration_PushOpeningIsRateLimitedPerAddress(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	addr, clientTLS := startTestServer(t, multihost.Deps{
		AgentStore: store.NewAgentStore(openIntegrationDB(t)), Broadcaster: noopBroadcaster{},
		Sessions: multihost.NewSessions(logger, noopBroadcaster{}), Logger: logger,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 1; i <= 11; i++ {
		stream, err := dialFrom(t, addr, clientTLS, "127.0.0.31").Push(ctx)
		require.NoError(t, err)
		msg, err := stream.Recv()
		if i <= 10 {
			require.NoError(t, err, "opening %d is within the quota", i)
			assert.NotNil(t, msg.GetChallenge())
			continue
		}
		require.Error(t, err, "opening %d is over the quota", i)
		assert.Equal(t, codes.ResourceExhausted, grpcstatus.Code(err), "%v", err)
		assert.Contains(t, grpcstatus.Convert(err).Message(), "too many connection attempts")
	}

	stream, err := dialFrom(t, addr, clientTLS, "127.0.0.32").Push(ctx)
	require.NoError(t, err)
	msg, err := stream.Recv()
	require.NoError(t, err, "another address keeps its own quota")
	assert.NotNil(t, msg.GetChallenge())
}

// US2 scenario 4, FR-013, SC-004: an unknown token is refused by a read, so 100 such
// calls answer while the writer is busy, and a valid enrollment follows unslowed.
func TestIntegration_UnknownTokenIsRefusedWithoutAWriteTransaction(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	db := openIntegrationDB(t)
	st := store.NewAgentStore(db)
	addr, clientTLS := startTestServer(t, multihost.Deps{AgentStore: st, Broadcaster: noopBroadcaster{}, Logger: logger})
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Hold the write transaction that any enrollment would have to wait for.
	started := make(chan struct{})
	release := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- db.Writer().Tx(ctx, func(ctx context.Context, tx *store.Tx) error {
			if err := tx.Serialize(ctx, "agents"); err != nil {
				return err
			}
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	codesSeen := map[codes.Code]int{}
	for i := range 100 {
		callCtx, callCancel := context.WithTimeout(ctx, 3*time.Second)
		client := dialFrom(t, addr, clientTLS, "127.0.0.41")
		_, err := client.RegisterAgent(callCtx, enrollRequest(fmt.Sprintf("a1b2c3d4-0000-4000-8000-0000000001%02d", i), "mnt_enr_neverissued", nil))
		callCancel()
		require.Error(t, err, "call %d", i)
		codesSeen[grpcstatus.Code(err)]++
		require.NotEqual(t, codes.DeadlineExceeded, grpcstatus.Code(err), "call %d waited on the writer: %v", i, err)
	}
	assert.GreaterOrEqual(t, codesSeen[codes.NotFound], 1, "unknown tokens are refused as NotFound: %v", codesSeen)
	assert.Equal(t, 100, codesSeen[codes.NotFound]+codesSeen[codes.ResourceExhausted], "only NotFound or the quota: %v", codesSeen)

	close(release)
	require.NoError(t, <-held)

	const token = "mnt_enr_validafterflood"
	storeToken(t, st, token)
	start := time.Now()
	_, err := dialFrom(t, addr, clientTLS, "127.0.0.42").RegisterAgent(ctx, enrollRequest("a1b2c3d4-0000-4000-8000-000000000042", token, nil))
	require.NoError(t, err, "a valid enrollment from another address goes through")
	assert.Less(t, time.Since(start), 2*time.Second, "the flood must not slow a valid enrollment down")
}

// US2 scenario 5, FR-014: field bounds and the UUID shape of agent_id are enforced
// before anything is written; the bounds themselves are accepted; the id is lowercased.
func TestIntegration_RegisterAgentValidatesFields(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	st := store.NewAgentStore(openIntegrationDB(t))
	addr, clientTLS := startTestServer(t, multihost.Deps{AgentStore: st, Broadcaster: noopBroadcaster{}, Logger: logger})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	refused := []struct {
		name  string
		edit  func(*agentpb.RegisterRequest)
		field string
	}{
		{"hostname of 254", func(r *agentpb.RegisterRequest) { r.Hostname = strings.Repeat("h", 254) }, "hostname"},
		{"os_arch of 33", func(r *agentpb.RegisterRequest) { r.OsArch = strings.Repeat("o", 33) }, "os_arch"},
		{"agent_version of 65", func(r *agentpb.RegisterRequest) { r.AgentVersion = strings.Repeat("v", 65) }, "agent_version"},
		{"agent_id not a uuid", func(r *agentpb.RegisterRequest) { r.AgentId = "agt-integration-001" }, "agent_id"},
		{"agent_id empty", func(r *agentpb.RegisterRequest) { r.AgentId = "" }, "agent_id"},
	}
	for i, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			token := fmt.Sprintf("mnt_enr_refused%02d", i)
			storeToken(t, st, token)
			req := enrollRequest(fmt.Sprintf("a1b2c3d4-0000-4000-8000-0000000002%02d", i), token, nil)
			c.edit(req)

			_, err := dialFrom(t, addr, clientTLS, "127.0.0.51").RegisterAgent(ctx, req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, grpcstatus.Code(err), "%v", err)
			assert.Contains(t, grpcstatus.Convert(err).Message(), c.field, "the message names the field")

			tok, err := st.GetByToken(ctx, token)
			require.NoError(t, err)
			assert.Nil(t, tok.ConsumedAt, "a refused request must not burn the token")
		})
	}

	accepted := []struct {
		name  string
		edit  func(*agentpb.RegisterRequest)
		check func(*testing.T, *agent.Agent)
	}{
		{"hostname of 253", func(r *agentpb.RegisterRequest) { r.Hostname = strings.Repeat("h", 253) },
			func(t *testing.T, a *agent.Agent) { assert.Equal(t, strings.Repeat("h", 253), a.Hostname) }},
		{"os_arch of 32", func(r *agentpb.RegisterRequest) { r.OsArch = strings.Repeat("o", 32) },
			func(t *testing.T, a *agent.Agent) { assert.Equal(t, strings.Repeat("o", 32), a.OSArch) }},
		{"agent_version of 64", func(r *agentpb.RegisterRequest) { r.AgentVersion = strings.Repeat("v", 64) },
			func(t *testing.T, a *agent.Agent) { assert.Equal(t, strings.Repeat("v", 64), a.AgentVersion) }},
	}
	for i, c := range accepted {
		t.Run(c.name, func(t *testing.T) {
			token := fmt.Sprintf("mnt_enr_accepted%02d", i)
			storeToken(t, st, token)
			id := fmt.Sprintf("a1b2c3d4-0000-4000-8000-0000000003%02d", i)
			req := enrollRequest(id, token, nil)
			c.edit(req)

			_, err := dialFrom(t, addr, clientTLS, "127.0.0.52").RegisterAgent(ctx, req)
			require.NoError(t, err, "the bound itself is accepted")
			a, err := st.Get(ctx, id)
			require.NoError(t, err)
			c.check(t, a)
		})
	}

	t.Run("uppercase agent_id is stored in lowercase", func(t *testing.T) {
		const token = "mnt_enr_uppercase0001"
		storeToken(t, st, token)
		const lower = "c0ffee00-1111-4222-8333-444455556666"
		_, err := dialFrom(t, addr, clientTLS, "127.0.0.52").RegisterAgent(ctx, enrollRequest(strings.ToUpper(lower), token, nil))
		require.NoError(t, err)
		a, err := st.Get(ctx, lower)
		require.NoError(t, err, "the agent is found by its lowercase id")
		assert.Equal(t, lower, a.AgentID)
		tok, err := st.GetByToken(ctx, token)
		require.NoError(t, err)
		require.NotNil(t, tok.ConsumedByAgentID)
		assert.Equal(t, lower, *tok.ConsumedByAgentID, "the token records the normalised id")
	})
}

// US2 scenario 6, FR-014 on Push: an uppercase agent_id authenticates the same agent;
// a non-UUID agent_id is refused as InvalidArgument.
func TestIntegration_PushAuthNormalisesTheAgentID(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	st := store.NewAgentStore(openIntegrationDB(t))
	sessions := multihost.NewSessions(logger, noopBroadcaster{})
	addr, clientTLS := startTestServer(t, multihost.Deps{
		AgentStore: st, Broadcaster: noopBroadcaster{}, Sessions: sessions, Limiter: multihost.NewLimiter(1000), Logger: logger,
	})
	client := dialGRPC(t, addr, clientTLS)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const lower = "d1e2f3a4-0000-4000-8000-0000000000aa"
	stream := authenticate(t, ctx, client, st, lower, strings.ToUpper(lower), "mnt_enr_pushupper001")
	require.Eventually(t, func() bool { return sessions.IsConnected(lower) }, 5*time.Second, 20*time.Millisecond,
		"the uppercase id must open the session of the lowercase agent")
	assert.False(t, sessions.IsConnected(strings.ToUpper(lower)), "the session is keyed on the normalised id")
	_ = stream.CloseSend()

	bad, err := client.Push(ctx)
	require.NoError(t, err)
	_, err = bad.Recv()
	require.NoError(t, err)
	require.NoError(t, bad.Send(&agentpb.ClientMessage{Payload: &agentpb.ClientMessage_Auth{Auth: &agentpb.AuthResponse{
		AgentId: "agt-not-a-uuid", Timestamp: time.Now().Unix(), Signature: make([]byte, 64),
	}}}))
	_, err = bad.Recv()
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcstatus.Code(err), "%v", err)
}

// awaitStreamError reads the stream until the server reports an error, or fails after timeout.
func awaitStreamError(t *testing.T, stream agentpb.Ingest_PushClient, timeout time.Duration) *agentpb.StreamError {
	t.Helper()
	got := make(chan *agentpb.StreamError, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				got <- nil
				return
			}
			if e := msg.GetError(); e != nil {
				got <- e
				return
			}
		}
	}()
	select {
	case e := <-got:
		require.NotNil(t, e, "the stream ended without a StreamError")
		return e
	case <-time.After(timeout):
		t.Fatal("no StreamError received")
		return nil
	}
}

func inventoryEvent(agentID string, seq uint64, inv *agentpb.ContainerInventory) *agentpb.ClientMessage {
	return &agentpb.ClientMessage{Payload: &agentpb.ClientMessage_Event{Event: &agentpb.AgentEvent{
		AgentId: agentID, EventId: fmt.Sprintf("evt-%d", seq), ObservedAt: timestamppb.Now(), Seq: seq,
		Body: &agentpb.AgentEvent_Inventory{Inventory: inv},
	}}}
}

func containersOf(n int, labels map[string]string) []*agentpb.ContainerEvent {
	out := make([]*agentpb.ContainerEvent, 0, n)
	for i := range n {
		out = append(out, &agentpb.ContainerEvent{
			ContainerId: fmt.Sprintf("c%05d", i), Name: fmt.Sprintf("n%d", i), Image: "app:v1",
			State: agentpb.ContainerState_CONTAINER_STATE_RUNNING, Labels: labels,
		})
	}
	return out
}

func labelsOf(n int) map[string]string {
	labels := make(map[string]string, n)
	for i := range n {
		labels[fmt.Sprintf("k%d", i)] = "v"
	}
	return labels
}

// US2 scenario 7, FR-015: over the real stream an inventory of 10 001 containers, or a
// container with 257 labels, is refused with a StreamError and nothing is written;
// 256 labels pass.
func TestIntegration_InventoryBoundsOverTheStream(t *testing.T) {
	withMultiHostEdition(t)
	logger := quietLogger()
	db := openIntegrationDB(t)
	st := store.NewAgentStore(db)
	containerStore := store.NewContainerStore(db)
	containerSvc := container.NewService(container.Deps{Store: containerStore, Logger: logger})
	sessions := multihost.NewSessions(logger, noopBroadcaster{})
	addr, clientTLS := startTestServer(t, multihost.Deps{
		AgentStore: st, Broadcaster: noopBroadcaster{}, Sessions: sessions, Limiter: multihost.NewLimiter(1000),
		Dispatcher: multihost.NewDispatcher(multihost.DispatchDeps{Container: containerSvc, Inventory: containerSvc}),
		Logger:     logger,
	})
	client := dialGRPC(t, addr, clientTLS)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const agentID = "e1e2f3a4-0000-4000-8000-0000000000bb"
	stream := authenticate(t, ctx, client, st, agentID, agentID, "mnt_enr_inventory0001")
	require.Eventually(t, func() bool { return sessions.IsConnected(agentID) }, 5*time.Second, 20*time.Millisecond)

	stored := func() int {
		list, err := containerStore.ListContainers(ctx, container.ListContainersOpts{IncludeIgnored: true, AgentFilter: ptr(agentID)})
		require.NoError(t, err)
		return len(list)
	}

	require.NoError(t, stream.Send(inventoryEvent(agentID, 1, &agentpb.ContainerInventory{Complete: true, Containers: containersOf(10_001, nil)})))
	e := awaitStreamError(t, stream, 10*time.Second)
	assert.Equal(t, "invalid_argument", e.GetCode(), "%v", e)
	assert.Equal(t, 0, stored(), "a refused inventory writes nothing")

	require.NoError(t, stream.Send(inventoryEvent(agentID, 2, &agentpb.ContainerInventory{Complete: true, Containers: containersOf(2, labelsOf(257))})))
	e = awaitStreamError(t, stream, 10*time.Second)
	assert.Equal(t, "invalid_argument", e.GetCode(), "%v", e)
	assert.Equal(t, 0, stored(), "a refused container writes nothing")

	require.NoError(t, stream.Send(inventoryEvent(agentID, 3, &agentpb.ContainerInventory{Complete: true, Containers: containersOf(2, labelsOf(256))})))
	require.Eventually(t, func() bool { return stored() == 2 }, 10*time.Second, 50*time.Millisecond, "256 labels are accepted")
}

func ptr[T any](v T) *T { return &v }

// US1 scenarios 4 and 5 through the agent binary, SC-002, SC-005: agent.Run with the
// server's fingerprint enrolls and streams over the persisted self-signed certificate
// without --grpc-insecure-skip-tls-verify; with another fingerprint nothing is enrolled.
func TestIntegration_AgentRunPinsTheServerCertificate(t *testing.T) {
	withMultiHostEdition(t)
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	serverLogger, _ := captureLogger()
	tlsCfg, err := multihost.PersistentSelfSigned(t.TempDir(), []string{"127.0.0.1"}, serverLogger)
	require.NoError(t, err)
	fp := sha256Hex(tlsCfg.Certificates[0].Certificate[0])

	st := store.NewAgentStore(openIntegrationDB(t))
	sessions := multihost.NewSessions(serverLogger, noopBroadcaster{})
	srv := multihost.New(multihost.Deps{
		AgentStore: st, Broadcaster: noopBroadcaster{}, Sessions: sessions, Limiter: multihost.NewLimiter(1000), Logger: serverLogger,
	})
	addr := freeAddr(t)
	srvCtx, stopServer := context.WithCancel(context.Background())
	t.Cleanup(stopServer)
	require.NoError(t, srv.Start(srvCtx, addr, tlsCfg))

	wrong := strings.Repeat("ab", 32)
	baseCfg := agent.AgentConfig{ServerURL: "grpcs://" + addr, RuntimeOverride: "docker", AgentVersion: "1.0.0-test"}

	t.Run("different fingerprint: nothing enrolled", func(t *testing.T) {
		const token = "mnt_enr_agentrunwrong01"
		storeToken(t, st, token)
		logger, logs := captureLogger()
		cfg := baseCfg
		cfg.DataDir = t.TempDir()
		cfg.EnrollmentToken = token
		cfg.ServerFingerprint = wrong

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := agent.Run(ctx, cfg, logger)
		require.Error(t, err, "the agent must not keep going against a server it cannot pin")
		agents, err := st.List(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, agents, "no enrollment reached the server")
		assert.Contains(t, logs.String(), fp, "the log names the fingerprint received")
		assert.Contains(t, logs.String(), wrong, "the log names the fingerprint expected")
	})

	t.Run("matching fingerprint: enrolled and streaming", func(t *testing.T) {
		const token = "mnt_enr_agentrunright01"
		storeToken(t, st, token)
		logger, logs := captureLogger()
		cfg := baseCfg
		cfg.DataDir = t.TempDir()
		cfg.EnrollmentToken = token
		cfg.ServerFingerprint = strings.ToUpper(fp)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- agent.Run(ctx, cfg, logger) }()

		require.Eventually(t, func() bool { return len(sessions.ListConnected()) == 1 }, 30*time.Second, 100*time.Millisecond,
			"the agent must authenticate its stream:\n%s", logs.String())
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(20 * time.Second):
			t.Fatal("agent.Run did not return after cancellation")
		}
		assert.NotContains(t, strings.ToLower(logs.String()), "verification is disabled", "pinning is not skipping")
	})
}
