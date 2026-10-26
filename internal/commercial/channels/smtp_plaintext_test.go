// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust/trusttest"
)

// Acceptance tests of 047-audit-small-fixes, C13: no mail leaves in clear without an explicit opt-in.

// FR-003, US1 scenario 3: a relay without STARTTLS on a port other than 465 gets nothing, and the error names the opt-in.
func TestSMTPSender_RefusesClearWithoutTheOptIn(t *testing.T) {
	for _, opt := range []struct {
		name string
		cfg  func(SMTPConfig) SMTPConfig
	}{
		{"variable absent", func(c SMTPConfig) SMTPConfig { return c }},
		{"variable false", func(c SMTPConfig) SMTPConfig { c.AllowPlaintext = false; return c }},
	} {
		t.Run(opt.name, func(t *testing.T) {
			host, port, data, rcpt := smtpStub(t)
			sender := NewSMTPSender(opt.cfg(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"}))

			err := sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MAINTENANT_SMTP_ALLOW_PLAINTEXT", "the operator must learn how to opt in")
			assert.Contains(t, err.Error(), "STARTTLS")
			assert.Contains(t, err.Error(), host, "the relay is named")

			time.Sleep(100 * time.Millisecond)
			assert.Empty(t, *rcpt, "no recipient may be announced in clear")
			assert.Empty(t, data.String(), "no body may leave in clear")
		})
	}
}

// FR-003, US1 scenario 3: MAINTENANT_SMTP_ALLOW_PLAINTEXT=true lets the mail go in clear.
func TestSMTPSender_SendsInClearWithTheOptIn(t *testing.T) {
	for _, tc := range []struct{ to, subject, body string }{
		{"ops@example.com", "disk full", "the disk is full"},
		{"visitor@example.org", "Confirm", "Open https://status.example.com/status/confirm?token=abc"},
	} {
		host, port, data, rcpt := smtpStub(t)
		sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com", AllowPlaintext: true})

		require.NoError(t, sender.Send(context.Background(), tc.to, tc.subject, tc.body))

		assert.Equal(t, []string{"<" + tc.to + ">"}, *rcpt)
		assert.Contains(t, data.String(), tc.body)
	}
}

// Edge case "SMTP sur le port 465": implicit TLS, the variable plays no part.
func TestSMTPSender_ImplicitTLSIgnoresTheOptIn(t *testing.T) {
	cert, ca := borrowedCertificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	delivered := make(chan string, 1)
	go serveOneMailOver(ln, cert, true, delivered)

	trusttest.Trust(t, ca)
	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com", AllowPlaintext: false})
	sender.implicitTLS = true

	require.NoError(t, sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full"))
	select {
	case body := <-delivered:
		assert.Contains(t, body, "the disk is full")
	case <-time.After(5 * time.Second):
		t.Fatal("no mail delivered over implicit TLS")
	}
}
