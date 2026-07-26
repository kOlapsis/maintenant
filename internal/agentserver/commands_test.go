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
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

const cmdAgentID = "agent-cmd"

func newCommandSessions(t *testing.T, caps []string) (*Sessions, chan *agentpb.ServerMessage) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := NewSessions(logger, nil)
	send := make(chan *agentpb.ServerMessage, 16)
	s.Open(cmdAgentID, func(error) {}, "addr", caps, send)
	return s, send
}

// respond plays the agent: it reads the queued command and answers on its id.
func respond(t *testing.T, s *Sessions, send chan *agentpb.ServerMessage, results ...*agentpb.CommandResult) string {
	t.Helper()
	select {
	case msg := <-send:
		cmd := msg.GetCommand()
		require.NotNil(t, cmd, "server must have queued an AgentCommand")
		for _, res := range results {
			res.RequestId = cmd.GetRequestId()
			s.DeliverResult(cmdAgentID, res)
		}
		return cmd.GetRequestId()
	case <-time.After(time.Second):
		t.Fatal("no command was queued for the agent")
		return ""
	}
}

func TestSessions_FetchLogs_CollectsChunksUntilLast(t *testing.T) {
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	done := make(chan []string, 1)
	go func() {
		lines, err := s.FetchLogs(context.Background(), cmdAgentID, "ctr", 100, false)
		assert.NoError(t, err)
		done <- lines
	}()

	respond(t, s, send,
		&agentpb.CommandResult{Result: &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{Lines: []string{"a", "b"}}}},
		&agentpb.CommandResult{Last: true, Result: &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{Lines: []string{"c"}}}},
	)

	select {
	case lines := <-done:
		assert.Equal(t, []string{"a", "b", "c"}, lines)
	case <-time.After(2 * time.Second):
		t.Fatal("FetchLogs did not return")
	}
}

func TestSessions_FetchLogs_PropagatesAgentError(t *testing.T) {
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	done := make(chan error, 1)
	go func() {
		_, err := s.FetchLogs(context.Background(), cmdAgentID, "ctr", 100, false)
		done <- err
	}()

	respond(t, s, send, &agentpb.CommandResult{
		ErrorCode: "logs_unavailable", ErrorMessage: "no such container", Last: true,
	})

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such container")
	case <-time.After(2 * time.Second):
		t.Fatal("FetchLogs did not return")
	}
}

func TestSessions_FetchLogs_UnknownAgentFailsFast(t *testing.T) {
	s, _ := newCommandSessions(t, []string{CapabilityLogs})

	_, err := s.FetchLogs(context.Background(), "nobody", "ctr", 100, false)
	assert.ErrorIs(t, err, ErrAgentNotConnected)
}

func TestSessions_FetchLogs_AgentWithoutCapabilityFailsFast(t *testing.T) {
	// An agent predating the command channel advertises nothing; the request must
	// be refused immediately rather than waiting out the timeout.
	s, _ := newCommandSessions(t, nil)

	start := time.Now()
	_, err := s.FetchLogs(context.Background(), cmdAgentID, "ctr", 100, false)

	assert.ErrorIs(t, err, ErrAgentCannotServe)
	assert.Less(t, time.Since(start), time.Second, "must not wait for a timeout")
}

func TestSessions_FetchLogs_DisconnectUnblocksWaiter(t *testing.T) {
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	done := make(chan error, 1)
	go func() {
		_, err := s.FetchLogs(context.Background(), cmdAgentID, "ctr", 100, false)
		done <- err
	}()

	<-send // the command was queued; the agent dies before answering
	s.Close(cmdAgentID, "stream_ended")

	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrAgentNotConnected)
	case <-time.After(2 * time.Second):
		t.Fatal("a disconnect must release the waiter instead of hanging it")
	}
}

func TestSessions_FetchLogs_HonoursContextDeadline(t *testing.T) {
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := s.FetchLogs(ctx, cmdAgentID, "ctr", 100, false)
		done <- err
	}()

	<-send // agent never answers

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("FetchLogs ignored its deadline")
	}
}

func TestSessions_SendCommand_RejectsBeyondInFlightCap(t *testing.T) {
	s, _ := newCommandSessions(t, []string{CapabilityLogs})

	for i := 0; i < maxInFlightPerAgent; i++ {
		_, _, err := s.SendCommand(context.Background(), cmdAgentID, CapabilityLogs,
			LogsCommand("ctr", 100, false, true))
		require.NoError(t, err, "request %d should be accepted", i)
	}

	_, _, err := s.SendCommand(context.Background(), cmdAgentID, CapabilityLogs,
		LogsCommand("ctr", 100, false, true))
	assert.ErrorIs(t, err, ErrTooManyRequests)
}

func TestSessions_Release_CancelsStillRunningRequest(t *testing.T) {
	// Closing a log view must tell the agent to stop tailing, or the follow leaks
	// on its side for as long as the container lives.
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	_, release, err := s.SendCommand(context.Background(), cmdAgentID, CapabilityLogs,
		LogsCommand("ctr", 100, false, true))
	require.NoError(t, err)

	logsMsg := <-send
	requestID := logsMsg.GetCommand().GetRequestId()

	release()

	select {
	case msg := <-send:
		cmd := msg.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, requestID, cmd.GetRequestId())
		assert.NotNil(t, cmd.GetCancel(), "release must send a CancelRequest for the same id")
	case <-time.After(time.Second):
		t.Fatal("no cancel was sent to the agent")
	}
}

func TestSessions_Release_NoCancelAfterCompletion(t *testing.T) {
	// A finished request owes the agent nothing; sending a stray cancel would be
	// pure noise on the stream.
	s, send := newCommandSessions(t, []string{CapabilityLogs})

	results, release, err := s.SendCommand(context.Background(), cmdAgentID, CapabilityLogs,
		LogsCommand("ctr", 100, false, false))
	require.NoError(t, err)

	msg := <-send
	s.DeliverResult(cmdAgentID, &agentpb.CommandResult{
		RequestId: msg.GetCommand().GetRequestId(), Last: true,
	})
	<-results // drain the terminal result

	release()

	select {
	case extra := <-send:
		t.Fatalf("unexpected message after completion: %v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSessions_HasCapability(t *testing.T) {
	s, _ := newCommandSessions(t, []string{CapabilityLogs})

	assert.True(t, s.HasCapability(cmdAgentID, CapabilityLogs))
	assert.False(t, s.HasCapability(cmdAgentID, "exec"))
	assert.False(t, s.HasCapability("nobody", CapabilityLogs))
}

func TestLogsCommand_ClampsTail(t *testing.T) {
	assert.Equal(t, uint32(100), LogsCommand("c", 0, false, false).GetLogs().GetLines())
	assert.Equal(t, uint32(100), LogsCommand("c", -5, false, false).GetLogs().GetLines())
	assert.Equal(t, uint32(500), LogsCommand("c", 10_000, false, false).GetLogs().GetLines())
	assert.Equal(t, uint32(42), LogsCommand("c", 42, false, false).GetLogs().GetLines())
}
