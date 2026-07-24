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

package agentserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
)

// ingestImpl implements agentpb.IngestServer.
type ingestImpl struct {
	agentpb.UnimplementedIngestServer
	deps Deps
}

// RegisterAgent handles first-boot agent enrollment.
func (impl *ingestImpl) RegisterAgent(ctx context.Context, req *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	logger := impl.deps.Logger

	// Resolve display label: if empty or >64 chars, fall back to hostname.
	label := req.GetLabel()
	if label == "" || len(label) > 64 {
		label = req.GetHostname()
	}
	// Final safety: truncate hostname to 64 chars if it somehow exceeds that.
	if len(label) > 64 {
		label = label[:64]
	}

	// Version compatibility check: block on major mismatch, warn on minor skew.
	checkVersionCompat(req.GetAgentVersion(), logger)

	// Enrol the agent atomically: the per-edition host cap check, the one-time
	// token consume, and the insert run in a single serialized transaction, so
	// concurrent enrollments can never both pass the cap and over-fill it. The
	// local runtime is never counted, and a rejected cap/token leaves the token
	// unconsumed. limit < 0 means unlimited.
	now := time.Now().UTC()
	a := &agent.Agent{
		AgentID:         req.GetAgentId(),
		PublicKey:       req.GetPublicKey(),
		Hostname:        req.GetHostname(),
		Label:           label,
		OSArch:          req.GetOsArch(),
		AgentVersion:    req.GetAgentVersion(),
		DetectedRuntime: protoRuntimeToString(req.GetDetectedRuntime()),
		Status:          "active",
		CreatedAt:       now,
	}
	if err := impl.deps.AgentStore.EnrollAtomic(ctx, extension.AgentHostLimit(), req.GetEnrollmentToken(), a); err != nil {
		switch {
		case errors.Is(err, agent.ErrHostLimitReached):
			logger.Warn("agent.enroll_rejected", "reason", "host_limit", "limit", extension.AgentHostLimit())
			return nil, grpcstatus.Error(codes.ResourceExhausted, "agent host limit reached")
		case errors.Is(err, agent.ErrTokenNotFound):
			return nil, grpcstatus.Error(codes.NotFound, "enrollment token not found")
		case errors.Is(err, agent.ErrTokenAlreadyConsumed):
			return nil, grpcstatus.Error(codes.FailedPrecondition, "enrollment token already consumed")
		case errors.Is(err, agent.ErrTokenExpired):
			return nil, grpcstatus.Error(codes.FailedPrecondition, "enrollment token expired")
		default:
			logger.Error("enroll agent failed", "agent_id", req.GetAgentId(), "err", err)
			return nil, grpcstatus.Error(codes.Internal, "failed to register agent")
		}
	}

	logger.Info("agent.registered",
		"agent_id", a.AgentID,
		"hostname", a.Hostname,
		"label", a.Label,
		"runtime", a.DetectedRuntime,
		"version", a.AgentVersion,
	)

	impl.deps.Broadcaster.BroadcastEvent(event.AgentCreated, map[string]any{
		"agent_id": a.AgentID,
		"hostname": a.Hostname,
		"label":    a.Label,
		"runtime":  a.DetectedRuntime,
		"status":   a.Status,
	})

	return &agentpb.RegisterResponse{
		ServerTime: timestamppb.New(now),
		AgentConfig: &agentpb.AgentConfig{
			ResourceSampleIntervalSeconds:  10,
			CertificateScanIntervalSeconds: 60,
			MaxEventsPerSecond:             1000,
		},
	}, nil
}

// Push implements the long-lived bidirectional event stream for an enrolled agent.
// Protocol: server sends AuthChallenge → client sends AuthResponse → server enters event loop.
func (impl *ingestImpl) Push(stream grpc.BidiStreamingServer[agentpb.ClientMessage, agentpb.ServerMessage]) error {
	logger := impl.deps.Logger
	sessions := impl.deps.Sessions
	limiter := impl.deps.Limiter
	dispatcher := impl.deps.Dispatcher

	// Phase 1: send auth challenge.
	challenge, err := GenerateChallenge()
	if err != nil {
		return grpcstatus.Error(codes.Internal, "failed to generate auth challenge")
	}
	if err := stream.Send(&agentpb.ServerMessage{
		Payload: &agentpb.ServerMessage_Challenge{Challenge: challenge},
	}); err != nil {
		return err
	}

	// Phase 2: receive auth response.
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	authResp, ok := msg.GetPayload().(*agentpb.ClientMessage_Auth)
	if !ok {
		return grpcstatus.Error(codes.InvalidArgument, "expected AuthResponse as first message")
	}
	resp := authResp.Auth

	// Load agent and verify signature.
	ag, err := impl.deps.AgentStore.Get(stream.Context(), resp.GetAgentId())
	if errors.Is(err, agent.ErrAgentNotFound) {
		return grpcstatus.Error(codes.NotFound, "agent not found")
	}
	if err != nil {
		logger.Error("Push: load agent failed", "agent_id", resp.GetAgentId(), "err", err)
		return grpcstatus.Error(codes.Internal, "load agent error")
	}

	if err := Verify(resp, challenge.Nonce, ag, time.Now()); err != nil {
		switch {
		case errors.Is(err, ErrAgentRevoked):
			logger.Warn("agent.auth_failed", "agent_id", ag.AgentID, "reason", "revoked")
			return grpcstatus.Error(codes.PermissionDenied, "agent_revoked")
		case errors.Is(err, ErrClockSkew):
			logger.Warn("agent.auth_failed", "agent_id", ag.AgentID, "reason", "clock_skew")
			return grpcstatus.Error(codes.DeadlineExceeded, "clock_skew")
		default:
			logger.Warn("agent.auth_failed", "agent_id", ag.AgentID, "reason", "bad_signature")
			return grpcstatus.Error(codes.Unauthenticated, "invalid signature")
		}
	}
	logger.Debug("agent.auth_ok", "agent_id", ag.AgentID)

	// Phase 3: open session with a cancellable context.
	// sessions.Close() calls cancel() which unblocks the recv loop below.
	addr := ""
	if p, ok := peer.FromContext(stream.Context()); ok {
		addr = p.Addr.String()
	}
	streamCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	if sessions != nil {
		sessions.Open(ag.AgentID, cancel, addr)
		defer sessions.Close(ag.AgentID, "stream_ended")
	}

	// Mark live on auth so the stale watcher can't reap a freshly (re)connected stream.
	connectedAt := time.Now()
	if err := impl.deps.AgentStore.UpdateLastSeen(stream.Context(), ag.AgentID, connectedAt); err != nil {
		logger.Error("Push: initial UpdateLastSeen failed", "agent_id", ag.AgentID, "err", err)
	}

	// Phase 4: async recv so we can select on session cancellation.
	type recvResult struct {
		msg *agentpb.ClientMessage
		err error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			recvCh <- recvResult{msg, err}
			if err != nil {
				return
			}
		}
	}()

	const ackEvery = 100
	var (
		eventCount  uint64
		lastSeenSeq uint64
		lastAck     = time.Now()
		lastSeenUpd = connectedAt
	)

	maybeAck := func(force bool) error {
		if !force && eventCount%ackEvery != 0 && time.Since(lastAck) < 5*time.Second {
			return nil
		}
		if err := stream.Send(&agentpb.ServerMessage{
			Payload: &agentpb.ServerMessage_Ack{
				Ack: &agentpb.EventAck{
					LastEventSeq: lastSeenSeq,
					ReceivedAt:   timestamppb.Now(),
				},
			},
		}); err != nil {
			return err
		}
		lastAck = time.Now()
		return nil
	}

	for {
		select {
		case <-streamCtx.Done():
			return grpcstatus.Error(codes.PermissionDenied, "agent_revoked")

		case res := <-recvCh:
			if res.err != nil {
				return res.err
			}
			evMsg, ok := res.msg.GetPayload().(*agentpb.ClientMessage_Event)
			if !ok {
				continue
			}
			evt := evMsg.Event

			if limiter != nil {
				if allowed, retryAfter := limiter.Allow(ag.AgentID); !allowed {
					retryMs := uint32(retryAfter.Milliseconds()) // #nosec G115 -- rate-limiter retry delay, always non-negative and sub-second
					logger.Warn("agent.rate_limited",
						"agent_id", ag.AgentID,
						"retry_after_ms", retryMs,
					)
					_ = stream.Send(&agentpb.ServerMessage{
						Payload: &agentpb.ServerMessage_Error{
							Error: &agentpb.StreamError{
								Code:         "rate_limited",
								Message:      "event rate limit exceeded",
								RetryAfterMs: retryMs,
							},
						},
					})
					continue
				}
			}

			eventCount++
			lastSeenSeq = evt.GetSeq()

			if time.Since(lastSeenUpd) > 20*time.Second {
				if err := impl.deps.AgentStore.UpdateLastSeen(stream.Context(), ag.AgentID, time.Now()); err != nil {
					logger.Error("Push: UpdateLastSeen failed", "agent_id", ag.AgentID, "err", err)
				}
				lastSeenUpd = time.Now()
			}

			if sessions != nil {
				sessions.IncrEvents(ag.AgentID)
			}

			if dispatcher != nil {
				if err := dispatcher.Dispatch(stream.Context(), evt); err != nil {
					logger.Error("Push: dispatch failed", "agent_id", ag.AgentID, "err", err)
				}
			}

			if err := maybeAck(false); err != nil {
				return err
			}
		}
	}
}

func protoRuntimeToString(rt agentpb.Runtime) string {
	switch rt {
	case agentpb.Runtime_RUNTIME_DOCKER:
		return "docker"
	case agentpb.Runtime_RUNTIME_SWARM:
		return "swarm"
	case agentpb.Runtime_RUNTIME_KUBERNETES:
		return "kubernetes"
	default:
		return "docker"
	}
}

func checkVersionCompat(agentVersion string, logger interface{ Warn(string, ...any) }) {
	if agentVersion == "" {
		return
	}
	// Major version is the first segment before the first dot.
	parts := strings.SplitN(agentVersion, ".", 2)
	major := parts[0]
	// Server is always v1.x at this time; reject 0.x or 2+.x
	if major != "1" && major != "0" {
		logger.Warn("agent version major mismatch — enrollment allowed but compatibility not guaranteed",
			"agent_version", agentVersion, "server_major", "1")
	}
}
