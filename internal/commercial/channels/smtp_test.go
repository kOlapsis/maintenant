// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mime"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust/trusttest"
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

func subjectHeader(t *testing.T, message string) string {
	t.Helper()
	for _, line := range strings.Split(message, "\n") {
		if value, ok := strings.CutPrefix(line, "Subject: "); ok {
			return strings.TrimRight(value, "\r")
		}
	}
	t.Fatalf("no Subject header in %q", message)
	return ""
}

func TestBuildMIMEEncodesANonASCIISubject(t *testing.T) {
	const subject = "Résolu : panne de la base de données"
	raw := subjectHeader(t, buildMIME("maintenant@example.com", "ops@example.com", subject, "body"))

	assert.True(t, strings.HasPrefix(raw, "=?utf-8?q?"), "RFC 2047 encoded word expected, got %q", raw)
	for _, r := range raw {
		require.Less(t, r, rune(0x80), "the header carries raw non-ASCII bytes: %q", raw)
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
	require.NoError(t, err)
	assert.Equal(t, subject, decoded)
}

func TestBuildMIMELeavesAnASCIISubjectReadable(t *testing.T) {
	assert.Equal(t, "[major] Database down",
		subjectHeader(t, buildMIME("maintenant@example.com", "ops@example.com", "[major] Database down", "body")))
}

func TestBuildMIMEKeepsHeaderInjectionOut(t *testing.T) {
	msg := buildMIME("maintenant@example.com", "ops@example.com", "Hello\r\nBcc: victim@example.com", "body")
	assert.NotContains(t, msg, "\r\nBcc:")
}

func TestSMTPSenderDeliversAnAccentedSubject(t *testing.T) {
	host, port, data, _ := smtpStub(t)
	s := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	require.NoError(t, s.Send(context.Background(), "visitor@example.com", "Mise à jour : réseau dégradé", "body"))

	decoded, err := new(mime.WordDecoder).DecodeHeader(subjectHeader(t, data.String()))
	require.NoError(t, err)
	assert.Equal(t, "Mise à jour : réseau dégradé", decoded)
}

func TestSMTPSenderDeliversPlainText(t *testing.T) {
	host, port, data, rcpt := smtpStub(t)
	s := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	require.NoError(t, s.Send(context.Background(), "visitor@example.com", "Confirm", "Open https://status.example.com/status/confirm?token=abc"))

	assert.Equal(t, []string{"<visitor@example.com>"}, *rcpt)
	assert.Contains(t, data.String(), "Content-Type: text/plain; charset=utf-8")
	assert.Contains(t, data.String(), "https://status.example.com/status/confirm?token=abc")
}

// serveOneMail plays a one-session STARTTLS relay and reports the bodies it accepted over TLS.
func serveOneMail(ln net.Listener, cert tls.Certificate, delivered chan<- string) {
	serveOneMailOver(ln, cert, false, delivered)
}

// serveOneMailOver plays one session, over TLS from the first byte when implicit, and reports the bodies it accepted over TLS.
func serveOneMailOver(ln net.Listener, cert tls.Certificate, implicit bool, delivered chan<- string) {
	raw, err := ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = raw.Close() }()

	conn := raw
	overTLS := false
	if implicit {
		tc := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		if err := tc.Handshake(); err != nil {
			return
		}
		conn, overTLS = tc, true
	}
	r := bufio.NewReader(conn)
	reply := func(line string) { _, _ = fmt.Fprintf(conn, "%s\r\n", line) }

	reply("220 relay ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if overTLS {
				reply("250 relay")
			} else {
				reply("250-relay")
				reply("250 STARTTLS")
			}
		case cmd == "STARTTLS":
			reply("220 go ahead")
			tc := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, r, overTLS = tc, bufio.NewReader(tc), true
		case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
			reply("250 ok")
		case cmd == "DATA":
			reply("354 end with .")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(l)
			}
			if overTLS {
				delivered <- body.String()
			}
			reply("250 queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func TestSMTPSender_StartTLSTrustsTheConfiguredCA(t *testing.T) {
	borrowed := httptest.NewTLSServer(nil)
	cert := borrowed.TLS.Certificates[0]
	ca := borrowed.Certificate()
	borrowed.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	delivered := make(chan string, 1)
	go serveOneMail(ln, cert, delivered)

	trusttest.Trust(t, ca)
	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	require.NoError(t, sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full"),
		"a relay signed by MAINTENANT_CA_CERT must be trusted")
	select {
	case body := <-delivered:
		assert.Contains(t, body, "the disk is full")
	case <-time.After(5 * time.Second):
		t.Fatal("no mail delivered over TLS")
	}
}

func borrowedCertificate(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	borrowed := httptest.NewTLSServer(nil)
	defer borrowed.Close()
	return borrowed.TLS.Certificates[0], borrowed.Certificate()
}

func TestSMTPSender_ImplicitTLSOnPort465(t *testing.T) {
	assert.True(t, NewSMTPSender(SMTPConfig{Host: "mail.example.com", Port: "465"}).implicitTLS)
	assert.False(t, NewSMTPSender(SMTPConfig{Host: "mail.example.com", Port: "587"}).implicitTLS)

	cert, ca := borrowedCertificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	delivered := make(chan string, 1)
	go serveOneMailOver(ln, cert, true, delivered)

	trusttest.Trust(t, ca)
	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})
	sender.implicitTLS = true

	require.NoError(t, sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full"))
	select {
	case body := <-delivered:
		assert.Contains(t, body, "the disk is full")
	case <-time.After(5 * time.Second):
		t.Fatal("no mail delivered over implicit TLS")
	}
}

// refusingStartTLSRelay offers STARTTLS, then turns it down, and records whether a message came through anyway.
func refusingStartTLSRelay(t *testing.T) (host, port string, gotData chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	gotData = make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		reply := func(line string) { _, _ = fmt.Fprintf(conn, "%s\r\n", line) }
		reply("220 relay ready")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				reply("250-relay")
				reply("250 STARTTLS")
			case cmd == "STARTTLS":
				reply("454 TLS not available due to temporary reason")
			case cmd == "DATA":
				gotData <- struct{}{}
				reply("354 end with .")
			case cmd == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	return host, port, gotData
}

func TestSMTPSender_AFailedStartTLSStopsTheMail(t *testing.T) {
	host, port, gotData := refusingStartTLSRelay(t)
	sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	err := sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starttls")
	select {
	case <-gotData:
		t.Fatal("the message went out in clear after STARTTLS failed")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSMTPSender_UntrustedStartTLSStopsTheMail(t *testing.T) {
	cert, _ := borrowedCertificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	delivered := make(chan string, 1)
	go serveOneMail(ln, cert, delivered)

	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	sender := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	err = sender.Send(context.Background(), "ops@example.com", "disk full", "the disk is full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starttls")
}
