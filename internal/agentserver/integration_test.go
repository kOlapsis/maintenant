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
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// noopBroadcaster satisfies agentserver.EventBroadcaster without side-effects.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastEvent(string, any) {}

// openIntegrationDB opens a temp SQLite DB with migrations and writer started.
func openIntegrationDB(t *testing.T) *sqlite.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := sqlite.Open(dbPath, logger)
	require.NoError(t, err, "open integration DB")

	err = sqlite.Migrate(db.ReadDB(), logger)
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

func TestIntegration_Enrollment(t *testing.T) {
	db := openIntegrationDB(t)
	store := sqlite.NewAgentStore(db)
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
	defer conn.Close()

	client := agentpb.NewIngestClient(conn)

	// Insert an enrollment token valid for 1 hour.
	tok := &agent.EnrollmentToken{
		TokenID:   "inttest001",
		Token:     "mnt_enr_integrationtest01",
		CreatedBy: "test",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, store.InsertToken(ctx, tok))

	// (a) First call should succeed and insert the agent.
	req := &agentpb.RegisterRequest{
		AgentId:         "agt-integration-001",
		EnrollmentToken: tok.Token,
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
	gotTok, err := store.GetByToken(ctx, tok.Token)
	require.NoError(t, err)
	assert.NotNil(t, gotTok.ConsumedAt, "token should be consumed")

	// (c) Re-call with same token → FailedPrecondition.
	req2 := &agentpb.RegisterRequest{
		AgentId:         "agt-integration-002",
		EnrollmentToken: tok.Token,
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
