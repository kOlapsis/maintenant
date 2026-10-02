// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/agentauth"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/retry"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/trust"
)

// ErrAgentRevokedServer is returned by RunWithReconnect when the server revokes the agent.
// The caller should exit without retrying.
var ErrAgentRevokedServer = errors.New("agent revoked by server")

// ErrAgentUnknownServer is returned by RunWithReconnect when the server has no record of the agent.
var ErrAgentUnknownServer = errors.New("agent unknown to the server, deleted or enrolled with another one")

// streamErrorRateLimited is the code the server sends when an agent pushes
// events faster than its per-agent allowance.
const streamErrorRateLimited = "rate_limited"

// StreamHooks lets the spool react to what the server says about the stream.
// A nil field ignores that signal.
type StreamHooks struct {
	Acked       func(uint64)
	RateLimited func(time.Duration)
}

// Client wraps the gRPC IngestClient with connection lifecycle management.
type Client struct {
	conn   *grpc.ClientConn
	client agentpb.IngestClient

	// commands and agentVersion are reported at auth and wired into every stream
	// this client opens. Set once via EnableCommands, before the first dial.
	commands     *CommandRunner
	agentVersion string
}

// EnableCommands lets the server issue commands (log reads) against rt, and makes
// the running build known at each connect. Must be called before DialPush; the
// receive loop starts inside it, so a later hook-up would race.
func (c *Client) EnableCommands(rt runtime.Runtime, agentVersion string, logger *slog.Logger) {
	c.commands = NewCommandRunner(rt, logger)
	c.agentVersion = agentVersion
}

var clientKeepalive = keepalive.ClientParameters{
	Time:                20 * time.Second,
	Timeout:             10 * time.Second,
	PermitWithoutStream: true,
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
		tlsCfg := trust.ClientTLSConfig()
		tlsCfg.InsecureSkipVerify = insecureSkipVerify // #nosec G402 -- explicit opt-in flag, warning logged at boot.
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
		grpc.WithKeepaliveParams(clientKeepalive),
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

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// PushStream wraps an authenticated bidirectional gRPC stream for pushing agent events.
// Multiple goroutines may call Send concurrently.
type PushStream struct {
	mu     sync.Mutex
	stream agentpb.Ingest_PushClient
	recvCh chan error
	done   chan struct{}

	// commands executes server-issued commands; nil disables the command channel.
	commands *CommandRunner
	hooks    StreamHooks
	// ctx bounds command work to the stream's lifetime.
	ctx context.Context
}

// Send wraps evt in a ClientMessage and delivers it unchanged.
func (ps *PushStream) Send(evt *agentpb.AgentEvent) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Event{Event: evt},
	})
}

// SendStatus reports the spool's state. It is not telemetry: the server neither
// rate-limits it nor dispatches it.
func (ps *PushStream) SendStatus(st *agentpb.SpoolStatus) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Status{Status: st},
	})
}

// SendResult delivers a reply to a server-issued command. Safe to call from the
// per-request goroutines, which share the stream with the telemetry senders.
func (ps *PushStream) SendResult(res *agentpb.CommandResult) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Result{Result: res},
	})
}

// Close signals the end of the send side of the stream.
func (ps *PushStream) Close() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	_ = ps.stream.CloseSend()
}

// Wait blocks until the receive goroutine exits and returns its error (nil on clean close).
// If ctx is cancelled before recvLoop exits, it returns ctx.Err() immediately so the caller
// can proceed with shutdown. The leaked recvLoop goroutine terminates when the underlying
// gRPC stream is finally closed (typically by the deferred grpcClient.Close()).
func (ps *PushStream) Wait(ctx context.Context) error {
	select {
	case err := <-ps.recvCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ps *PushStream) recvLoop(logger *slog.Logger) {
	var retErr error
	for {
		msg, err := ps.stream.Recv()
		if err != nil {
			retErr = err
			break
		}
		if errMsg := msg.GetError(); errMsg != nil {
			if logger != nil {
				logger.Warn("agent: server error",
					"code", errMsg.GetCode(),
					"message", errMsg.GetMessage(),
					"retry_after_ms", errMsg.GetRetryAfterMs(),
				)
			}
			if errMsg.GetCode() == streamErrorRateLimited && ps.hooks.RateLimited != nil {
				ps.hooks.RateLimited(time.Duration(errMsg.GetRetryAfterMs()) * time.Millisecond)
			}
		}
		if ack := msg.GetAck(); ack != nil && ps.hooks.Acked != nil {
			ps.hooks.Acked(ack.GetLastEventSeq())
		}
		if cmd := msg.GetCommand(); cmd != nil && ps.commands != nil {
			ps.commands.Handle(ps.ctx, ps, cmd)
		}
	}
	// The stream is gone; abandon work whose requester can no longer be reached.
	if ps.commands != nil {
		ps.commands.CancelAll()
	}
	ps.recvCh <- retErr
	close(ps.done)
}

// DialPush opens the bidirectional Push stream, performs the Ed25519 auth handshake,
// and returns a PushStream ready to send events.
func (c *Client) DialPush(ctx context.Context, id *Identity, logger *slog.Logger, hooks StreamHooks) (*PushStream, error) {
	stream, err := c.client.Push(ctx)
	if err != nil {
		return nil, fmt.Errorf("open Push stream: %w", err)
	}

	// Phase 1: receive auth challenge from server.
	msg, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("recv auth challenge: %w", err)
	}
	challengePayload, ok := msg.GetPayload().(*agentpb.ServerMessage_Challenge)
	if !ok {
		return nil, fmt.Errorf("expected AuthChallenge as first server message")
	}
	nonce := challengePayload.Challenge.GetNonce()

	// Phase 2: build and sign the payload (byte format shared with the server
	// via internal/agentauth).
	now := time.Now().Unix()
	payload, err := agentauth.BuildSignPayload(nonce, id.AgentID, now)
	if err != nil {
		return nil, fmt.Errorf("build sign payload: %w", err)
	}

	// Phase 3: send AuthResponse. Version and capabilities ride along outside the
	// signed payload, so old and new servers both accept this message.
	if err := stream.Send(&agentpb.ClientMessage{
		Payload: &agentpb.ClientMessage_Auth{
			Auth: &agentpb.AuthResponse{
				AgentId:      id.AgentID,
				Timestamp:    now,
				Signature:    id.Sign(payload),
				AgentVersion: c.agentVersion,
				Capabilities: Capabilities(),
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("send auth response: %w", err)
	}

	ps := &PushStream{
		stream:   stream,
		recvCh:   make(chan error, 1),
		done:     make(chan struct{}),
		commands: c.commands,
		hooks:    hooks,
		ctx:      ctx,
	}
	go ps.recvLoop(logger)

	return ps, nil
}

// RunWithReconnect runs onStream with exponential backoff reconnect.
// Returns nil when ctx is cancelled, ErrAgentRevokedServer or ErrAgentUnknownServer
// when the server refuses the identity.
// Backoff: min(60s, 1s * 2^attempt) ±25% jitter. Attempt resets to 0 if stream was stable >30s.
func RunWithReconnect(
	ctx context.Context,
	c *Client,
	id *Identity,
	logger *slog.Logger,
	hooks StreamHooks,
	onStream func(ctx context.Context, stream *PushStream) error,
) error {
	const stable = 30 * time.Second
	backoff := retry.New(time.Second, 60*time.Second, 0.25)

	for {
		if ctx.Err() != nil {
			return nil
		}

		start := time.Now()
		stream, dialErr := c.DialPush(ctx, id, logger, hooks)
		if dialErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			if refusal := refusedIdentity(dialErr); refusal != nil {
				logger.Error("agent: identity refused during dial", "err", dialErr)
				return refusal
			}
			logger.Warn("agent: push dial failed", "err", dialErr, "attempt", backoff.Attempt())
		} else {
			// A sender only notices a closed stream on its next write, which may never come.
			streamCtx, stopStream := context.WithCancel(ctx)
			go func() {
				select {
				case <-stream.done:
					stopStream()
				case <-streamCtx.Done():
				}
			}()
			streamErr := onStream(streamCtx, stream)
			stopStream()
			stream.Close()
			// The server reports revocation on the receive side; the send side only
			// ever surfaces a generic EOF. Ignoring this error turned a revocation
			// into an endless reconnect loop.
			recvErr := stream.Wait(ctx)

			if time.Since(start) > stable {
				backoff.Reset()
			}

			if ctx.Err() != nil {
				return nil
			}
			if refusal := refusedIdentity(errors.Join(streamErr, recvErr)); refusal != nil {
				logger.Error("agent: identity refused by server", "err", errors.Join(streamErr, recvErr))
				return refusal
			}
			if streamErr != nil || recvErr != nil {
				logger.Warn("agent: stream closed, will reconnect",
					"err", errors.Join(streamErr, recvErr), "attempt", backoff.Attempt())
			}
		}

		delay := backoff.Next()
		logger.Info("agent: reconnecting", "delay", delay.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// refusedIdentity maps a server refusal of the stored identity to its sentinel, or returns nil.
func refusedIdentity(err error) error {
	for _, e := range flatten(err) {
		var carrier interface{ GRPCStatus() *grpcstatus.Status }
		if !errors.As(e, &carrier) {
			continue
		}
		st := carrier.GRPCStatus()
		switch {
		case st.Code() == codes.PermissionDenied && st.Message() == "agent_revoked":
			return ErrAgentRevokedServer
		case st.Code() == codes.NotFound && st.Message() == "agent not found":
			return ErrAgentUnknownServer
		}
	}
	return nil
}

func flatten(err error) []error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}
	return []error{err}
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
		// Bare host:port — assume TLS.
		return u, true, nil
	}
}
