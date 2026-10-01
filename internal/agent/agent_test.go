// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// recordingIngest enrolls anyone and keeps every event an agent pushes.
type recordingIngest struct {
	agentpb.UnimplementedIngestServer

	mu     sync.Mutex
	events []*agentpb.AgentEvent
}

func (s *recordingIngest) RegisterAgent(context.Context, *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	return &agentpb.RegisterResponse{}, nil
}

func (s *recordingIngest) Push(stream grpc.BidiStreamingServer[agentpb.ClientMessage, agentpb.ServerMessage]) error {
	if err := stream.Send(&agentpb.ServerMessage{
		Payload: &agentpb.ServerMessage_Challenge{Challenge: &agentpb.AuthChallenge{Nonce: make([]byte, 32)}},
	}); err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil
		}
		if ev := msg.GetEvent(); ev != nil {
			s.mu.Lock()
			s.events = append(s.events, ev)
			s.mu.Unlock()
		}
	}
}

func (s *recordingIngest) received() []*agentpb.AgentEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*agentpb.AgentEvent(nil), s.events...)
}

func TestRun_StartsOnAHostWithoutContainerRuntime(t *testing.T) {
	prev := resourceSampleInterval
	resourceSampleInterval = 10 * time.Millisecond
	t.Cleanup(func() { resourceSampleInterval = prev })
	t.Setenv("DOCKER_HOST", "unix://"+t.TempDir()+"/docker.sock")

	srv := &recordingIngest{}
	cfg := AgentConfig{
		DataDir:             t.TempDir(),
		ServerURL:           startIngest(t, srv, nil),
		EnrollmentToken:     "tok",
		RuntimeOverride:     RuntimeDocker,
		AgentVersion:        "1.0.0",
		SpoolMaxMemoryBytes: 1 << 20,
		SpoolMaxDiskBytes:   1 << 24,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, quietLogger()) }()

	require.Eventually(t, func() bool {
		var hostOS, hostSample bool
		for _, ev := range srv.received() {
			if ev.GetHostOs() != nil {
				hostOS = true
			}
			if r := ev.GetResource(); r != nil && r.GetContainerId() == "" {
				hostSample = true
			}
		}
		return hostOS && hostSample
	}, 10*time.Second, 20*time.Millisecond, "an agent without Docker must still enroll and report its host")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}
