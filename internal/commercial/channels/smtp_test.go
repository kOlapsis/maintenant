// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// silentServer accepts connections and never says a word.
func silentServer(t *testing.T) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	return host, port
}

func TestSMTPSenderGivesUpWhenItsContextEnds(t *testing.T) {
	host, port := silentServer(t)
	s := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Send(ctx, "ops@example.com", "subject", "body") }()

	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Send kept waiting on a silent server past its context")
	}
}

func TestSMTPSenderDeliversPlainText(t *testing.T) {
	host, port, data, rcpt := smtpStub(t)
	s := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	require.NoError(t, s.Send(context.Background(), "visitor@example.com", "Confirm", "Open https://status.example.com/status/confirm?token=abc"))

	assert.Equal(t, []string{"<visitor@example.com>"}, *rcpt)
	assert.Contains(t, data.String(), "Content-Type: text/plain; charset=utf-8")
	assert.Contains(t, data.String(), "https://status.example.com/status/confirm?token=abc")
}
