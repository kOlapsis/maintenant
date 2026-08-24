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

// Package agentserver implements the gRPC Ingest service that remote agents
// connect to for enrollment and event streaming (Pro only).
package agentserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/store"
)

// EventBroadcaster is the minimal interface required to publish SSE events to connected clients.
type EventBroadcaster interface {
	BroadcastEvent(eventType string, data any)
}

// Deps groups the dependencies required by the agent gRPC server.
type Deps struct {
	AgentStore  *store.AgentStore
	Broadcaster EventBroadcaster
	Sessions    *Sessions
	Limiter     *Limiter
	Dispatcher  *Dispatcher
	Logger      *slog.Logger
}

// Server wraps the gRPC server lifecycle for the Ingest service.
type Server struct {
	grpc *grpc.Server
	impl *ingestImpl
	deps Deps
}

// New creates a Server. Call Start to begin accepting connections.
func New(deps Deps) *Server {
	return &Server{
		impl: &ingestImpl{deps: deps},
		deps: deps,
	}
}

// Start binds to listen, registers the Ingest service, and begins serving.
// It blocks until the context is cancelled or an error occurs.
// If tlsCfg is nil the server listens in h2c (plaintext) — only safe behind a
// trusted reverse proxy (MAINTENANT_GRPC_TLS_INSECURE=true).
func (s *Server) Start(ctx context.Context, listen string, tlsCfg *tls.Config) error {
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("agentserver: listen %s: %w", listen, err)
	}

	// Recovery interceptors turn any handler panic into a gRPC Internal error
	// instead of crashing the process. gRPC does not recover panics by default,
	// so without this a single malformed request in any handler is a full DoS.
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(recoveryUnaryInterceptor(s.deps.Logger)),
		grpc.ChainStreamInterceptor(recoveryStreamInterceptor(s.deps.Logger)),
	}
	if tlsCfg != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}
	s.grpc = grpc.NewServer(opts...)
	agentpb.RegisterIngestServer(s.grpc, s.impl)

	s.deps.Logger.Info("agent gRPC server starting", "listen", listen)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.grpc.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		s.grpc.GracefulStop()
		return nil
	case err := <-errCh:
		return fmt.Errorf("agentserver: serve: %w", err)
	}
}

// Stop performs a graceful shutdown of the gRPC server.
func (s *Server) Stop() {
	if s.grpc != nil {
		s.grpc.GracefulStop()
	}
}

// recoveryUnaryInterceptor recovers any panic raised by a unary handler,
// logs it with a stack trace, and returns a generic Internal error.
func recoveryUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("agentserver: recovered from panic",
					"method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = grpcstatus.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// recoveryStreamInterceptor is the streaming counterpart of
// recoveryUnaryInterceptor: it keeps a panic in a stream handler from taking
// the whole server down.
func recoveryStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("agentserver: recovered from panic",
					"method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = grpcstatus.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}

// StartTokenGC launches a background goroutine that purges unconsumed enrollment
// tokens older than 7 days. It ticks every hour until ctx is cancelled.
func (s *Server) StartTokenGC(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.deps.AgentStore.GcExpiredTokens(ctx); err != nil {
					s.deps.Logger.Error("enrollment token GC failed", "err", err)
				}
			}
		}
	}()
}
