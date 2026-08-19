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
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
)

// noopBroadcaster satisfies agentserver.EventBroadcaster without side-effects.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastEvent(string, any) {}

// captureBroadcaster records every BroadcastEvent call for assertions.
type captureBroadcaster struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	eventType string
	data      any
}

func (b *captureBroadcaster) BroadcastEvent(eventType string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, capturedEvent{eventType, data})
}

func (b *captureBroadcaster) hasEventType(t string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.eventType == t {
			return true
		}
	}
	return false
}

// withMultiHostEdition enables the edition that allows agent enrollment, which
// the Community host cap otherwise refuses.
func withMultiHostEdition(t *testing.T) {
	t.Helper()
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })
}

// openIntegrationDB opens a temp SQLite DB with migrations and writer started.
func openIntegrationDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := store.Open(dbPath, logger)
	require.NoError(t, err, "open integration DB")

	err = store.Migrate(db.ReadDB(), logger)
	require.NoError(t, err, "run migrations on integration DB")

	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)
	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
	return db
}

// selfSignedTLS generates a self-signed TLS certificate valid for 127.0.0.1.
func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

// buildSignPayload mirrors agentserver/auth.go Verify for test use.
func buildSignPayload(nonce []byte, agentID string, ts int64) ([]byte, error) {
	clean := strings.ReplaceAll(agentID, "-", "")
	uuidBytes, err := hex.DecodeString(clean)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 56)
	copy(payload[0:32], nonce)
	copy(payload[32:48], uuidBytes)
	binary.BigEndian.PutUint64(payload[48:56], uint64(ts))
	return payload, nil
}

// startTestServer starts a gRPC server on a random loopback port and returns its
// address plus the client TLS config that trusts its self-signed certificate.
// Pass the config straight to dialGRPC.
func startTestServer(t *testing.T, deps agentserver.Deps) (listenAddr string, clientTLS *tls.Config) {
	t.Helper()
	tlsCfg := selfSignedTLS(t)
	srv := agentserver.New(deps)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr = lis.Addr().String()
	_ = lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = srv.Start(ctx, listenAddr, tlsCfg) }()
	time.Sleep(50 * time.Millisecond)

	// Trust the server's own self-signed leaf; nothing else would validate.
	certPool := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(tlsCfg.Certificates[0].Certificate[0])
	require.NoError(t, err)
	certPool.AddCert(leaf)

	return listenAddr, &tls.Config{RootCAs: certPool, ServerName: "127.0.0.1"} // #nosec G402 -- loopback test server, pinned to its own self-signed leaf
}

func dialGRPC(t *testing.T, addr string, tlsCfg *tls.Config) agentpb.IngestClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return agentpb.NewIngestClient(conn)
}

func TestIntegration_Enrollment(t *testing.T) {
	withMultiHostEdition(t)

	db := openIntegrationDB(t)
	store := store.NewAgentStore(db)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Start the gRPC server on a random loopback port.
	srv := agentserver.New(agentserver.Deps{
		AgentStore:  store,
		Broadcaster: noopBroadcaster{},
		Logger:      logger,
	})
	tlsCfg := selfSignedTLS(t)

	// Bind to port 0 to get an OS-assigned port; then pass the actual addr to Start.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := lis.Addr().String()
	_ = lis.Close() // Release so Start can rebind — tiny race window is fine in tests.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx, listenAddr, tlsCfg)
	}()

	// Give the server a moment to bind.
	time.Sleep(50 * time.Millisecond)

	// Build a gRPC client that skips cert verification (self-signed).
	certPool := x509.NewCertPool()
	certDER := tlsCfg.Certificates[0].Certificate[0]
	leaf, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	certPool.AddCert(leaf)

	clientTLS := &tls.Config{RootCAs: certPool, ServerName: "127.0.0.1"}
	conn, err := grpc.NewClient(listenAddr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := agentpb.NewIngestClient(conn)

	// Insert an enrollment token valid for 1 hour. Only its hash is stored; the
	// cleartext below is what the client sends.
	const tokCleartext = "mnt_enr_integrationtest01"
	require.NoError(t, store.InsertToken(ctx, &agent.EnrollmentToken{
		TokenID:     "inttest001",
		TokenHash:   agent.HashToken(tokCleartext),
		TokenPrefix: agent.TokenPrefix(tokCleartext),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}))

	// (a) First call should succeed and insert the agent.
	req := &agentpb.RegisterRequest{
		AgentId:         "agt-integration-001",
		EnrollmentToken: tokCleartext,
		Hostname:        "testhost",
		Label:           "Test Agent",
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       make([]byte, 32),
	}

	resp, err := client.RegisterAgent(ctx, req)
	require.NoError(t, err, "first RegisterAgent call should succeed")
	assert.NotNil(t, resp.GetServerTime())
	assert.NotNil(t, resp.GetAgentConfig())

	// (b) Verify agent is in the DB.
	gotAgent, err := store.Get(ctx, req.AgentId)
	require.NoError(t, err, "agent should be inserted in DB")
	assert.Equal(t, req.Hostname, gotAgent.Hostname)
	assert.Equal(t, "active", gotAgent.Status)

	// (b) Verify token is marked consumed.
	gotTok, err := store.GetByToken(ctx, tokCleartext)
	require.NoError(t, err)
	assert.NotNil(t, gotTok.ConsumedAt, "token should be consumed")

	// (c) Re-call with same token → FailedPrecondition.
	req2 := &agentpb.RegisterRequest{
		AgentId:         "agt-integration-002",
		EnrollmentToken: tokCleartext,
		Hostname:        "testhost2",
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       make([]byte, 32),
	}
	_, err = client.RegisterAgent(ctx, req2)
	require.Error(t, err, "second call with same token should fail")
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code(), "should be FailedPrecondition for already-consumed token")

	// Ensure no server-side errors.
	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err, "server should exit cleanly on context cancel")
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down within 2s")
	}
}

// TestIntegration_PushStream validates the full US2 flow:
// enroll → open stream → auth handshake → push container event →
// DB persistence with agent_id → SSE broadcast (SC-002).
func TestIntegration_PushStream(t *testing.T) {
	withMultiHostEdition(t)

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	db := openIntegrationDB(t)
	agentStore := store.NewAgentStore(db)
	containerStore := store.NewContainerStore(db)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	broadcaster := &captureBroadcaster{}

	containerSvc := container.NewService(container.Deps{
		Store:  containerStore,
		Logger: logger,
		EventCallback: func(eventType string, data interface{}) {
			broadcaster.BroadcastEvent(eventType, data)
		},
	})

	sessions := agentserver.NewSessions(logger, broadcaster)
	limiter := agentserver.NewLimiter(1000)
	dispatcher := agentserver.NewDispatcher(agentserver.DispatchDeps{Container: containerSvc})

	tlsCfg := selfSignedTLS(t)
	srv := agentserver.New(agentserver.Deps{
		AgentStore:  agentStore,
		Broadcaster: broadcaster,
		Sessions:    sessions,
		Limiter:     limiter,
		Dispatcher:  dispatcher,
		Logger:      logger,
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := lis.Addr().String()
	_ = lis.Close()

	go func() { _ = srv.Start(ctx, listenAddr, tlsCfg) }()
	time.Sleep(60 * time.Millisecond)

	certPool := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(tlsCfg.Certificates[0].Certificate[0])
	require.NoError(t, err)
	certPool.AddCert(leaf)
	clientTLS := &tls.Config{RootCAs: certPool, ServerName: "127.0.0.1"}
	conn, err := grpc.NewClient(listenAddr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	grpcClient := agentpb.NewIngestClient(conn)

	// === Step 1: Enroll agent with a real Ed25519 keypair ===
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Use a proper UUID-format agent ID (32 hex chars with dashes after stripping).
	agentID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

	const pushTok = "mnt_enr_pushtest00000001"
	require.NoError(t, agentStore.InsertToken(ctx, &agent.EnrollmentToken{
		TokenID:     "push-test-tok",
		TokenHash:   agent.HashToken(pushTok),
		TokenPrefix: agent.TokenPrefix(pushTok),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}))

	_, err = grpcClient.RegisterAgent(ctx, &agentpb.RegisterRequest{
		AgentId:         agentID,
		EnrollmentToken: pushTok,
		Hostname:        "push-test-host",
		OsArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       []byte(pub),
	})
	require.NoError(t, err, "enroll should succeed")

	// === Step 2: Pre-insert a container with State=Exited so handleStateChange can process a "start" ===
	const testExternalID = "cnt-integration-push-001"
	_, err = containerStore.InsertContainer(ctx, &container.Container{
		ExternalID:        testExternalID,
		Name:              "test-push-container",
		Image:             "alpine:latest",
		State:             container.StateExited,
		AgentID:           agentID,
		AlertSeverity:     container.SeverityInfo,
		FirstSeenAt:       time.Now().UTC(),
		LastStateChangeAt: time.Now().UTC(),
		RuntimeType:       "docker",
	})
	require.NoError(t, err, "pre-insert container")

	// === Step 3: Open Push stream and perform auth handshake ===
	stream, err := grpcClient.Push(ctx)
	require.NoError(t, err, "open Push stream")

	// Receive AuthChallenge.
	srvMsg, err := stream.Recv()
	require.NoError(t, err, "recv auth challenge")
	challenge, ok := srvMsg.GetPayload().(*agentpb.ServerMessage_Challenge)
	require.True(t, ok, "first message must be AuthChallenge")
	nonce := challenge.Challenge.GetNonce()

	// Build and sign payload.
	now := time.Now().Unix()
	payload, err := buildSignPayload(nonce, agentID, now)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, payload)

	err = stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Auth{
			Auth: &agentpb.AuthResponse{
				AgentId:   agentID,
				Timestamp: now,
				Signature: sig,
			},
		},
	})
	require.NoError(t, err, "send auth response")

	// === Step 4: Push a CONTAINER_STATE_RUNNING event ===
	err = stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Event{
			Event: &agentpb.AgentEvent{
				AgentId:    agentID,
				EventId:    "evt-push-001",
				ObservedAt: timestamppb.Now(),
				Seq:        1,
				Body: &agentpb.AgentEvent_Container{
					Container: &agentpb.ContainerEvent{
						ContainerId: testExternalID,
						Name:        "test-push-container",
						State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
						Labels:      map[string]string{},
					},
				},
			},
		},
	})
	require.NoError(t, err, "send container event")

	// === Step 5: Verify DB — container state changed to Running (< 4s) ===
	require.Eventually(t, func() bool {
		c, err := containerSvc.GetContainerByExternalID(ctx, testExternalID)
		if err != nil || c == nil {
			return false
		}
		return c.State == container.StateRunning
	}, 4*time.Second, 30*time.Millisecond, "container state should be Running in DB within 4s (SC-002)")

	// === Step 6: Verify SSE container.state_changed was broadcast ===
	// Waited for, not asserted outright: the service persists the state before it
	// emits, so step 5 can win the race and leave the event still in flight.
	require.Eventually(t, func() bool {
		return broadcaster.hasEventType(event.ContainerStateChanged)
	}, 4*time.Second, 30*time.Millisecond, "container.state_changed SSE should be broadcast")

	// Verify agent_id key is present in the SSE payload.
	broadcaster.mu.Lock()
	var ssePayload map[string]interface{}
	for _, e := range broadcaster.events {
		if e.eventType == event.ContainerStateChanged {
			if m, ok := e.data.(map[string]interface{}); ok {
				ssePayload = m
				break
			}
		}
	}
	broadcaster.mu.Unlock()
	require.NotNil(t, ssePayload, "state_changed payload should be a map")
	_, hasAgentID := ssePayload["agent_id"]
	assert.True(t, hasAgentID, "SSE payload should contain agent_id key")
}

// TestIntegration_LogsCommandRoundTrip drives the server→agent command channel over
// a real gRPC stream: the HTTP-side caller enqueues a command, the Push loop writes
// it to the stream, the agent answers, and the reply reaches the waiter.
//
// This is the wiring unit tests cannot reach — the send queue drained by Push's
// select, and CommandResult routed back out of the receive branch.
func TestIntegration_LogsCommandRoundTrip(t *testing.T) {
	withMultiHostEdition(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := openIntegrationDB(t)
	agentStore := store.NewAgentStore(db)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sessions := agentserver.NewSessions(logger, noopBroadcaster{})
	listenAddr, clientTLS := startTestServer(t, agentserver.Deps{
		AgentStore:  agentStore,
		Broadcaster: noopBroadcaster{},
		Sessions:    sessions,
		Limiter:     agentserver.NewLimiter(1000),
		Logger:      logger,
	})
	grpcClient := dialGRPC(t, listenAddr, clientTLS)

	// === Enroll ===
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	const agentID = "c0ffee00-1111-2222-3333-444455556666"

	const cmdTok = "mnt_enr_cmdtest000000001"
	require.NoError(t, agentStore.InsertToken(ctx, &agent.EnrollmentToken{
		TokenID:     "cmd-test-tok",
		TokenHash:   agent.HashToken(cmdTok),
		TokenPrefix: agent.TokenPrefix(cmdTok),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}))

	_, err = grpcClient.RegisterAgent(ctx, &agentpb.RegisterRequest{
		AgentId:         agentID,
		EnrollmentToken: cmdTok,
		Hostname:        "cmd-test-host",
		OsArch:          "linux/amd64",
		AgentVersion:    "1.3.7",
		DetectedRuntime: agentpb.Runtime_RUNTIME_DOCKER,
		PublicKey:       []byte(pub),
	})
	require.NoError(t, err)

	// === Auth, advertising the logs capability and a newer build ===
	stream, err := grpcClient.Push(ctx)
	require.NoError(t, err)

	srvMsg, err := stream.Recv()
	require.NoError(t, err)
	challenge, ok := srvMsg.GetPayload().(*agentpb.ServerMessage_Challenge)
	require.True(t, ok)

	now := time.Now().Unix()
	payload, err := buildSignPayload(challenge.Challenge.GetNonce(), agentID, now)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Auth{Auth: &agentpb.AuthResponse{
			AgentId:      agentID,
			Timestamp:    now,
			Signature:    ed25519.Sign(priv, payload),
			AgentVersion: "1.4.0",
			Capabilities: []string{agentserver.CapabilityLogs},
		}},
	}))

	require.Eventually(t, func() bool {
		return sessions.HasCapability(agentID, agentserver.CapabilityLogs)
	}, 4*time.Second, 20*time.Millisecond, "the server must record the advertised capability")

	// The running build must replace the version frozen at enrollment.
	require.Eventually(t, func() bool {
		a, err := agentStore.Get(ctx, agentID)
		return err == nil && a.AgentVersion == "1.4.0"
	}, 4*time.Second, 20*time.Millisecond, "agent_version must be refreshed from AuthResponse")

	// === Agent side: answer any logs command it receives ===
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			cmd := msg.GetCommand()
			if cmd == nil || cmd.GetLogs() == nil {
				continue
			}
			_ = stream.Send(&agentpb.ClientMessage{
				Payload: &agentpb.ClientMessage_Result{Result: &agentpb.CommandResult{
					RequestId: cmd.GetRequestId(),
					Last:      true,
					Result: &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{
						Lines: []string{"hello from " + cmd.GetLogs().GetContainerId()},
					}},
				}},
			})
		}
	}()

	// === Server side: the API layer asks for a tail ===
	lines, err := sessions.FetchLogs(ctx, agentID, "ctr-xyz", 100, false)
	require.NoError(t, err, "the command must round-trip over the real stream")
	assert.Equal(t, []string{"hello from ctr-xyz"}, lines)

	// An agent that never advertised the capability is refused, not timed out.
	_, err = sessions.FetchLogs(ctx, "11111111-0000-0000-0000-000000000000", "ctr", 100, false)
	assert.ErrorIs(t, err, agentserver.ErrAgentNotConnected)
}
