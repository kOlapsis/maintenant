// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

// DialInspect opens a TLS connection that accepts any certificate, so a chain
// can be reported even when it is invalid, and returns its state.
// DialInspect completes a TLS handshake that accepts any certificate and returns the connection state, without sending a single application byte.
func DialInspect(ctx context.Context, addr, serverName string, timeout time.Duration) (tls.ConnectionState, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return tls.ConnectionState{}, err
	}

	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- inspection dial: invalid certs must be retrieved to be reported; chain is validated by the caller.
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS10, // inspection dial: reach legacy hosts down to TLS 1.0
	})

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return tls.ConnectionState{}, err
	}

	state := tlsConn.ConnectionState()
	_ = conn.Close()
	return state, nil
}
