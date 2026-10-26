// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelay_ReachesServerThroughSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := gomcp.NewServer(&gomcp.Implementation{Name: "test", Version: "0"}, nil)
	gomcp.AddTool(server, &gomcp.Tool{Name: "ping"}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "pong"}}}, nil, nil
	})

	path := filepath.Join(t.TempDir(), "mcp.sock")
	require.NoError(t, ServeSocket(ctx, server, path, slog.Default()))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	clientToRelay, relayIn := io.Pipe()
	relayOut, clientFromRelay := io.Pipe()
	relayDone := make(chan error, 1)
	go func() {
		relayDone <- Relay(ctx, path, clientToRelay, clientFromRelay)
		_ = clientFromRelay.Close()
	}()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &gomcp.IOTransport{Reader: relayOut, Writer: relayIn}, nil)
	require.NoError(t, err)

	res, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: "ping"})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	assert.Equal(t, "pong", res.Content[0].(*gomcp.TextContent).Text)

	require.NoError(t, cs.Close())
	select {
	case err := <-relayDone:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not return after the client closed")
	}
}

func TestRelay_NoInstance(t *testing.T) {
	err := Relay(context.Background(), filepath.Join(t.TempDir(), "mcp.sock"), nil, io.Discard)
	assert.ErrorContains(t, err, "no running maintenant instance")
}
