// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDialInspectControl_SilentPeerFailsWithinTheTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, err := DialInspectControl(context.Background(), ln.Addr().String(), "example.test", 200*time.Millisecond, nil)
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("DialInspectControl did not return within 3s against a peer that never answers the handshake")
	}
}
