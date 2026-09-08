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
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

func startKeepaliveServer(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	srv := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Start(ctx, addr, nil)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("agent gRPC server did not stop within 5s")
		}
	})

	waitDialable(t, addr)
	return addr
}

func waitDialable(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s never became dialable", addr)
}

// A rejected call still proves the transport is alive; a dead transport surfaces as codes.Unavailable.
func register(ctx context.Context, c agentpb.IngestClient) error {
	_, err := c.RegisterAgent(ctx, &agentpb.RegisterRequest{})
	return err
}

var agentKeepalive = keepalive.ClientParameters{
	Time:                20 * time.Second,
	Timeout:             10 * time.Second,
	PermitWithoutStream: true,
}

func TestKeepalive_PolicyIsPermissive(t *testing.T) {
	require.True(t, serverKeepalivePolicy.PermitWithoutStream,
		"an idle agent must not be punished for pinging without a stream")
	require.LessOrEqual(t, serverKeepalivePolicy.MinTime, agentKeepalive.Time,
		"our own agent must ping slower than the server's enforcement floor")
}

func TestKeepalive_ClientWithoutKeepaliveIsNeverDropped(t *testing.T) {
	addr := startKeepaliveServer(t)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := agentpb.NewIngestClient(conn)

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := register(ctx, client)
		cancel()
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err),
			"call %d failed at the transport level: %v", i, err)
		time.Sleep(50 * time.Millisecond)
	}

	require.Equal(t, connectivity.Ready, conn.GetState(),
		"connection left READY without the client ever sending a keepalive ping")
}

func TestKeepalive_DeadStreamDetected(t *testing.T) {
	budget := agentKeepalive.Time + agentKeepalive.Timeout
	require.LessOrEqual(t, budget, 40*time.Second,
		"agent keepalive budget must detect a dead stream well under 40s")

	if testing.Short() {
		t.Skip("black-hole check costs ~12s: grpc-go clamps client ping intervals to 10s")
	}

	addr := startKeepaliveServer(t)
	proxy := startFreezableProxy(t, addr)

	// grpc-go silently raises any client ping interval below 10s back to it.
	scaled := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             time.Second,
		PermitWithoutStream: agentKeepalive.PermitWithoutStream,
	}
	conn, err := grpc.NewClient(proxy.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(scaled),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := agentpb.NewIngestClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(register(ctx, client)))

	proxy.freeze()

	start := time.Now()
	callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer callCancel()
	err = register(callCtx, client)
	elapsed := time.Since(start)
	require.Equal(t, codes.Unavailable, grpcstatus.Code(err),
		"a black-holed connection must fail with Unavailable, got %v", err)
	require.Less(t, elapsed, scaled.Time+scaled.Timeout+5*time.Second)
}

type freezableProxy struct {
	addr   string
	frozen atomic.Bool
}

func startFreezableProxy(t *testing.T, target string) *freezableProxy {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &freezableProxy{addr: lis.Addr().String()}

	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = lis.Close()
		wg.Wait()
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			in, err := lis.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				_ = in.Close()
				continue
			}
			wg.Add(2)
			go func() { defer wg.Done(); p.pipe(in, out) }()
			go func() { defer wg.Done(); p.pipe(out, in) }()
		}
	}()

	return p
}

func (p *freezableProxy) freeze() { p.frozen.Store(true) }

func (p *freezableProxy) pipe(src, dst net.Conn) {
	defer func() { _ = src.Close(); _ = dst.Close() }()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 && !p.frozen.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				return
			}
			return
		}
	}
}
