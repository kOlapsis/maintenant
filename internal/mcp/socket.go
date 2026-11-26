// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SocketPath returns the Unix socket the running instance serves MCP on, next to its database.
func SocketPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "mcp.sock")
}

// ServeSocket serves server on a Unix socket readable only by the instance's user until ctx ends, closing every session then, and returns a function that waits for the sessions to end.
func ServeSocket(ctx context.Context, server *gomcp.Server, path string, logger *slog.Logger) (func(), error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale MCP socket: %w", err)
	}
	old := syscall.Umask(0o077)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("listen on MCP socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("restrict MCP socket: %w", err)
	}

	var sessions sync.WaitGroup
	sessions.Go(func() {
		<-ctx.Done()
		_ = ln.Close()
	})
	sessions.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			sessions.Go(func() {
				defer func() { _ = conn.Close() }()
				// A tool call in flight still runs to its end: the SDK does not cancel it when the connection closes.
				stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
				defer stop()
				ss, err := server.Connect(ctx, &gomcp.IOTransport{Reader: conn, Writer: conn}, nil)
				if err != nil {
					logger.Warn("MCP socket session failed", "error", err)
					return
				}
				_ = ss.Wait()
			})
		}
	})
	return sessions.Wait, nil
}

// Relay pipes a stdio MCP client to the socket of the running instance, until either side closes.
func Relay(ctx context.Context, path string, in io.Reader, out io.Writer) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf("no running maintenant instance serves MCP at %s (start it with MAINTENANT_MCP=true or MAINTENANT_MCP_SOCKET=true): %w", path, err)
	}
	defer func() { _ = conn.Close() }()

	go func() {
		_, _ = io.Copy(conn, in)
		_ = conn.(*net.UnixConn).CloseWrite()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(out, conn)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}
