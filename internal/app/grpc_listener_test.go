// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/commercial"
	"github.com/kolapsis/maintenant/internal/extension"
)

type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestStart_CommunityExplainsWhyNoAgentListener(t *testing.T) {
	withEdition(t, extension.Community)
	cfg, _ := modeGateCfg(t, "embedded")
	sink := &logSink{}
	logger := slog.New(slog.NewTextHandler(sink, &slog.HandlerOptions{Level: slog.LevelInfo}))

	a, err := app.New(cfg, logger, app.WithExtensions(commercial.Extensions()))
	require.NoError(t, err)
	_ = startAndCollect(t, a, 2*time.Second)

	out := sink.String()
	assert.Contains(t, out, "agent gRPC listener not started")
	assert.Contains(t, out, "required_edition="+string(extension.MinEdition(extension.CapMultihost)))
	assert.Contains(t, out, "edition="+string(extension.Community))
	assert.NotContains(t, out, "agent gRPC server listening")
}

func TestStart_ServesAgentsWhereMultihostIsOpen(t *testing.T) {
	withEdition(t, extension.Pro)
	cfg, logger := modeGateCfg(t, "server")

	a, err := app.New(cfg, logger, app.WithExtensions(commercial.Extensions()))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", cfg.MultiHost.GRPCListen, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 10*time.Second, 100*time.Millisecond, "the commercial build must serve agents where multi-host is open")
}

func TestStart_RefusesHalfAGRPCKeypair(t *testing.T) {
	withEdition(t, extension.Pro)
	cfg, logger := modeGateCfg(t, "server")
	cfg.MultiHost.InsecureGRPC = false
	cfg.MultiHost.TLSCertFile = "/etc/maintenant/grpc.crt"

	a, err := app.New(cfg, logger, app.WithExtensions(commercial.Extensions()))
	require.NoError(t, err)

	err = startAndCollect(t, a, 5*time.Second)
	require.ErrorIs(t, err, app.ErrGRPCTLSPair, "half a keypair must stop startup instead of serving a self-signed certificate")
	assert.Contains(t, err.Error(), "MAINTENANT_GRPC_TLS_KEY")
}
