// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/extpoint"
)

type multiHost struct {
	server *Server
	logger *slog.Logger
}

// NewMultiHost builds the agent session registry and gRPC server; app serves it only where the plan allows multi-host.
func NewMultiHost(d extpoint.MultiHostDeps) extpoint.MultiHost {
	sessions := NewSessions(d.Logger, d.Broadcaster)
	m := &multiHost{
		server: New(Deps{
			AgentStore:  d.AgentStore,
			Sessions:    sessions,
			Broadcaster: d.Broadcaster,
			Limiter:     NewLimiter(d.RateLimitPerSecond),
			DemoMode:    d.DemoMode,
			Dispatcher: NewDispatcher(DispatchDeps{
				Container:   d.Container,
				Inventory:   d.Container,
				Resource:    d.Resource,
				Endpoint:    d.Endpoint,
				Certificate: d.Certificate,
				Heartbeat:   d.Heartbeat,
				Swarm:       d.Swarm,
				Kubernetes:  d.Kubernetes,
				HostOS:      d.HostOS,
				Runtime:     runtimeRecorder{store: d.AgentStore, broadcaster: d.Broadcaster},
				LabelSync:   d.LabelSync,
			}),
			Logger: d.Logger.With("component", "agentserver"),
		}),
		logger: d.Logger,
	}
	return extpoint.MultiHost{Sessions: sessions, Serve: m.serve}
}

// StartWatchers starts the ring-buffer tick and the stale-agent watcher.
func (s *Sessions) StartWatchers(ctx context.Context, staleThreshold time.Duration, staleAgents func(ctx context.Context, threshold time.Duration) ([]string, error)) {
	s.StartRingAdvancer(ctx)
	s.StartStaleWatcher(ctx, 10*time.Second, staleThreshold, OfflineReportGrace, staleAgents)
}

// serve binds and serves the agent-facing gRPC server in a background
// goroutine. TLS is required (FR-031); if no keypair is configured a
// self-signed dev cert is generated in-memory and a warning is logged.
func (m *multiHost) serve(ctx context.Context, cfg extpoint.GRPCConfig) error {
	listen := cfg.Listen
	if listen == "" {
		listen = "127.0.0.1:8443"
	}

	var tlsCfg *tls.Config
	if cfg.Insecure {
		m.logger.Warn("agentserver: TLS disabled — only use behind a trusted reverse proxy (MAINTENANT_GRPC_TLS_INSECURE)")
	} else {
		hosts := collectGRPCTLSHosts(cfg.PublicURL, listen)
		var err error
		tlsCfg, err = LoadOrGenerateTLS(
			cfg.TLSCertFile,
			cfg.TLSKeyFile,
			hosts,
			m.logger.With("component", "agentserver"),
		)
		if err != nil {
			return err
		}
	}

	m.server.StartTokenGC(ctx)
	if err := m.server.Start(ctx, listen, tlsCfg); err != nil {
		return err
	}
	m.logger.Info("agent gRPC server listening", "listen", listen)
	return nil
}

// collectGRPCTLSHosts returns the SAN list used for the self-signed dev TLS
// cert. It pulls the host out of the public URL (when set) and the listen
// address; wildcards like 0.0.0.0/:: are filtered. Empty result is handled
// downstream by falling back to 127.0.0.1 + localhost.
func collectGRPCTLSHosts(publicURL, listen string) []string {
	var hosts []string
	add := func(raw string) {
		if raw == "" {
			return
		}
		h := raw
		if hh, _, err := net.SplitHostPort(raw); err == nil {
			h = hh
		}
		if h == "" || h == "0.0.0.0" || h == "::" || h == "[::]" {
			return
		}
		hosts = append(hosts, h)
	}

	if publicURL != "" {
		stripped := publicURL
		for _, scheme := range []string{"grpcs://", "grpc://", "https://", "http://"} {
			if rest, ok := strings.CutPrefix(stripped, scheme); ok {
				stripped = rest
				break
			}
		}
		// stripped may carry a trailing path/query — keep only the authority.
		if i := strings.IndexAny(stripped, "/?#"); i >= 0 {
			stripped = stripped[:i]
		}
		add(stripped)
	}
	add(listen)
	return hosts
}
