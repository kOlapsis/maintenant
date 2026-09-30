// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/trust/trusttest"
)

type fakeIngest struct {
	agentpb.UnimplementedIngestServer
	pushErr    error
	validToken string

	pushes   atomic.Int32
	mu       sync.Mutex
	enrolled []string
}

func (f *fakeIngest) RegisterAgent(_ context.Context, req *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	if req.GetEnrollmentToken() != f.validToken {
		return nil, grpcstatus.Error(codes.FailedPrecondition, "enrollment token already consumed")
	}
	f.mu.Lock()
	f.enrolled = append(f.enrolled, req.GetAgentId())
	f.mu.Unlock()
	return &agentpb.RegisterResponse{}, nil
}

func (f *fakeIngest) Push(stream grpc.BidiStreamingServer[agentpb.ClientMessage, agentpb.ServerMessage]) error {
	f.pushes.Add(1)
	if err := stream.Send(&agentpb.ServerMessage{
		Payload: &agentpb.ServerMessage_Challenge{Challenge: &agentpb.AuthChallenge{Nonce: make([]byte, 32)}},
	}); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return f.pushErr
}

func (f *fakeIngest) enrolledIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.enrolled...)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startIngest serves srv on loopback, over TLS when cert is set, and returns its agent URL.
func startIngest(t *testing.T, srv agentpb.IngestServer, cert *tls.Certificate) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var opts []grpc.ServerOption
	scheme := "grpc://"
	if cert != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{*cert}, MinVersion: tls.VersionTLS12})))
		scheme = "grpcs://"
	}
	s := grpc.NewServer(opts...)
	agentpb.RegisterIngestServer(s, srv)
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return scheme + ln.Addr().String()
}

func dialIngest(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(context.Background(), url, false, quietLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRefusedIdentity(t *testing.T) {
	revoked := grpcstatus.Error(codes.PermissionDenied, "agent_revoked")
	unknown := grpcstatus.Error(codes.NotFound, "agent not found")
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"revoked", revoked, ErrAgentRevokedServer},
		{"unknown", unknown, ErrAgentUnknownServer},
		{"wrapped", fmt.Errorf("recv auth challenge: %w", unknown), ErrAgentUnknownServer},
		{"joined after a transport error", errors.Join(io.EOF, revoked), ErrAgentRevokedServer},
		{"session closed", grpcstatus.Error(codes.Unavailable, "session_closed"), nil},
		{"unknown enrollment token", grpcstatus.Error(codes.NotFound, "enrollment token not found"), nil},
		{"demo mode", grpcstatus.Error(codes.PermissionDenied, "demo mode: this server does not accept agents"), nil},
		{"not a status", io.EOF, nil},
		{"nil", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, refusedIdentity(c.err))
		})
	}
}

// blockUntilDone is a drain with nothing to send: only the receive side can end it.
func blockUntilDone(ctx context.Context, _ *PushStream) error {
	<-ctx.Done()
	return nil
}

func TestRunWithReconnect_StopsWhenTheServerRefusesTheIdentity(t *testing.T) {
	for _, c := range []struct {
		name    string
		pushErr error
		want    error
	}{
		{"deleted agent", grpcstatus.Error(codes.NotFound, "agent not found"), ErrAgentUnknownServer},
		{"revoked agent", grpcstatus.Error(codes.PermissionDenied, "agent_revoked"), ErrAgentRevokedServer},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := &fakeIngest{pushErr: c.pushErr}
			client := dialIngest(t, startIngest(t, srv, nil))
			id, err := newIdentity()
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err = RunWithReconnect(ctx, client, id, quietLogger(), StreamHooks{}, blockUntilDone)

			require.ErrorIs(t, err, c.want)
			assert.Equal(t, int32(1), srv.pushes.Load(), "a refused identity must not be retried")
		})
	}
}

func TestRunWithReconnect_KeepsRetryingAnUnavailableServer(t *testing.T) {
	srv := &fakeIngest{pushErr: grpcstatus.Error(codes.Unavailable, "session_closed")}
	client := dialIngest(t, startIngest(t, srv, nil))
	id, err := newIdentity()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunWithReconnect(ctx, client, id, quietLogger(), StreamHooks{}, blockUntilDone)
	}()

	require.Eventually(t, func() bool { return srv.pushes.Load() >= 2 }, 10*time.Second, 50*time.Millisecond,
		"an outage must be retried with the backoff")
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithReconnect did not return after cancellation")
	}
}

type serveScript struct {
	results []error
	served  []string
}

func (s *serveScript) serve(_ context.Context, id *Identity) error {
	s.served = append(s.served, id.AgentID)
	if len(s.served) > len(s.results) {
		return nil
	}
	return s.results[len(s.served)-1]
}

func registeredIdentity(t *testing.T, dir string) *Identity {
	t.Helper()
	id, err := LoadOrCreate(dir)
	require.NoError(t, err)
	id.Registered = true
	require.NoError(t, id.Save(dir))
	return id
}

func TestEnrollAndServe_ReenrollsWithTheConfiguredToken(t *testing.T) {
	for _, refusal := range []error{ErrAgentRevokedServer, ErrAgentUnknownServer} {
		t.Run(refusal.Error(), func(t *testing.T) {
			dir := t.TempDir()
			old := registeredIdentity(t, dir)
			srv := &fakeIngest{validToken: "fresh-token"}
			client := dialIngest(t, startIngest(t, srv, nil))
			cfg := AgentConfig{DataDir: dir, EnrollmentToken: "fresh-token"}
			enroll := func(ctx context.Context, id *Identity) error {
				return RunEnrollment(ctx, id, dir, cfg.EnrollmentToken, RuntimeDocker, "", "1.0.0", client)
			}
			script := &serveScript{results: []error{refusal}}

			require.NoError(t, enrollAndServe(context.Background(), cfg, old, enroll, script.serve, quietLogger()))

			require.Len(t, script.served, 2)
			assert.Equal(t, old.AgentID, script.served[0])
			assert.NotEqual(t, old.AgentID, script.served[1], "the refused identity must be replaced")
			assert.Equal(t, []string{script.served[1]}, srv.enrolledIDs())

			stored, err := LoadOrCreate(dir)
			require.NoError(t, err)
			assert.Equal(t, script.served[1], stored.AgentID, "the new identity must survive a restart")
			assert.True(t, stored.Registered)
		})
	}
}

func TestEnrollAndServe_StopsWithoutTokenWhenRefused(t *testing.T) {
	dir := t.TempDir()
	old := registeredIdentity(t, dir)
	enrolls := 0
	enroll := func(context.Context, *Identity) error { enrolls++; return nil }
	script := &serveScript{results: []error{ErrAgentRevokedServer}}

	err := enrollAndServe(context.Background(), AgentConfig{DataDir: dir}, old, enroll, script.serve, quietLogger())

	require.ErrorIs(t, err, ErrAgentRevokedServer)
	assert.Contains(t, err.Error(), "MAINTENANT_ENROLLMENT_TOKEN", "the message must say how to recover")
	assert.Contains(t, err.Error(), old.AgentID)
	assert.Zero(t, enrolls)
	assert.Len(t, script.served, 1, "no retry loop on a refused identity")
}

func TestEnrollAndServe_KeepsStoredIdentityWhenReenrollmentFails(t *testing.T) {
	dir := t.TempDir()
	old := registeredIdentity(t, dir)
	srv := &fakeIngest{validToken: "fresh-token"}
	client := dialIngest(t, startIngest(t, srv, nil))
	cfg := AgentConfig{DataDir: dir, EnrollmentToken: "consumed-token"}
	enroll := func(ctx context.Context, id *Identity) error {
		return RunEnrollment(ctx, id, dir, cfg.EnrollmentToken, RuntimeDocker, "", "1.0.0", client)
	}
	script := &serveScript{results: []error{ErrAgentUnknownServer}}

	err := enrollAndServe(context.Background(), cfg, old, enroll, script.serve, quietLogger())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already consumed")
	assert.Len(t, script.served, 1)
	stored, lerr := LoadOrCreate(dir)
	require.NoError(t, lerr)
	assert.Equal(t, old.AgentID, stored.AgentID, "a failed re-enrollment must not destroy the stored identity")
}

func TestEnrollAndServe_ReenrollsOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	old := registeredIdentity(t, dir)
	enrolls := 0
	enroll := func(_ context.Context, id *Identity) error {
		enrolls++
		id.Registered = true
		return nil
	}
	script := &serveScript{results: []error{ErrAgentRevokedServer, ErrAgentRevokedServer}}

	err := enrollAndServe(context.Background(), AgentConfig{DataDir: dir, EnrollmentToken: "tok"}, old, enroll, script.serve, quietLogger())

	require.ErrorIs(t, err, ErrAgentRevokedServer)
	assert.Equal(t, 1, enrolls)
	assert.Len(t, script.served, 2)
}

func TestEnrollAndServe_OtherOutcomesPassThrough(t *testing.T) {
	dir := t.TempDir()
	old := registeredIdentity(t, dir)
	boom := errors.New("boom")
	enroll := func(context.Context, *Identity) error { t.Fatal("no enrollment expected"); return nil }

	for _, res := range []error{nil, boom} {
		script := &serveScript{results: []error{res}}
		err := enrollAndServe(context.Background(), AgentConfig{DataDir: dir, EnrollmentToken: "tok"}, old, enroll, script.serve, quietLogger())
		assert.Equal(t, res, err)
		assert.Len(t, script.served, 1)
	}
}

func TestNewClient_TrustsTheConfiguredCA(t *testing.T) {
	tlsSrv := httptest.NewTLSServer(nil)
	cert := tlsSrv.TLS.Certificates[0]
	x509Cert := tlsSrv.Certificate()
	tlsSrv.Close()

	srv := &fakeIngest{validToken: "tok"}
	url := startIngest(t, srv, &cert)
	req := &agentpb.RegisterRequest{AgentId: "a", EnrollmentToken: "tok"}

	_, err := dialIngest(t, url).Register(context.Background(), req)
	require.Error(t, err, "a server signed by an unknown authority must be refused")

	trusttest.Trust(t, x509Cert)
	_, err = dialIngest(t, url).Register(context.Background(), req)
	require.NoError(t, err, "a server signed by MAINTENANT_CA_CERT must be trusted")
}
