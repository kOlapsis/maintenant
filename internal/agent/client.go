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

package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// Client wraps the gRPC IngestClient with connection lifecycle management.
type Client struct {
	conn   *grpc.ClientConn
	client agentpb.IngestClient
}

// NewClient dials the server at serverURL and returns a ready-to-use Client.
// serverURL should start with "grpcs://" (TLS) or "grpc://" (plaintext, not recommended).
// If insecureSkipVerify is true, TLS certificate validation is skipped (debug only).
func NewClient(ctx context.Context, serverURL string, insecureSkipVerify bool, logger *slog.Logger) (*Client, error) {
	target, useTLS, err := parseServerURL(serverURL)
	if err != nil {
		return nil, err
	}

	var creds credentials.TransportCredentials
	if useTLS {
		tlsCfg := &tls.Config{
			InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // explicit opt-in with warning at boot
		}
		if insecureSkipVerify {
			logger.Warn("TLS certificate verification is disabled — do not use in production")
		}
		creds = credentials.NewTLS(tlsCfg)
	} else {
		logger.Warn("connecting to agent server without TLS — plaintext gRPC")
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("dial agent server %s: %w", target, err)
	}

	return &Client{
		conn:   conn,
		client: agentpb.NewIngestClient(conn),
	}, nil
}

// Register calls the unary RegisterAgent RPC.
func (c *Client) Register(ctx context.Context, req *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	resp, err := c.client.RegisterAgent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RegisterAgent: %w", err)
	}
	return resp, nil
}

// Push opens the bidirectional streaming RPC.
func (c *Client) Push(ctx context.Context) (agentpb.Ingest_PushClient, error) {
	stream, err := c.client.Push(ctx)
	if err != nil {
		return nil, fmt.Errorf("Push: %w", err)
	}
	return stream, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// parseServerURL extracts the host:port target and whether TLS should be used.
// Accepted schemes: grpcs:// (TLS), grpc:// (plaintext), or bare host:port (defaults to TLS).
func parseServerURL(u string) (target string, useTLS bool, err error) {
	switch {
	case strings.HasPrefix(u, "grpcs://"):
		return strings.TrimPrefix(u, "grpcs://"), true, nil
	case strings.HasPrefix(u, "grpc://"):
		return strings.TrimPrefix(u, "grpc://"), false, nil
	case strings.HasPrefix(u, "https://"):
		return strings.TrimPrefix(u, "https://"), true, nil
	case u == "":
		return "", false, fmt.Errorf("server URL is empty")
	default:
		// Bare host:port — assume TLS
		return u, true, nil
	}
}
