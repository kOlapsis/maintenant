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
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/runtime"
)

// captureSender records the results the runner produces.
type captureSender struct {
	mu      sync.Mutex
	results []*agentpb.CommandResult
	err     error
}

func (c *captureSender) SendResult(res *agentpb.CommandResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.results = append(c.results, res)
	return nil
}

func (c *captureSender) snapshot() []*agentpb.CommandResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*agentpb.CommandResult(nil), c.results...)
}

func (c *captureSender) lines() []string {
	var out []string
	for _, r := range c.snapshot() {
		out = append(out, r.GetLogs().GetLines()...)
	}
	return out
}

// trackingReader reports whether the runner closed it.
type trackingReader struct {
	io.Reader
	closed atomic.Bool
}

func (t *trackingReader) Close() error {
	t.closed.Store(true)
	return nil
}

// logRuntime is a runtime.Runtime that only implements the log methods; every
// other call panics, which keeps the fake honest about what the runner touches.
type logRuntime struct {
	runtime.Runtime
	fetch      []string
	fetchErr   error
	streamData io.ReadCloser
	streamErr  error

	blockStream chan struct{}
}

func (l *logRuntime) FetchLogs(_ context.Context, _ string, _ int, _ bool) ([]string, error) {
	return l.fetch, l.fetchErr
}

func (l *logRuntime) StreamLogs(_ context.Context, _ string, _ int, _ bool) (io.ReadCloser, error) {
	if l.blockStream != nil {
		<-l.blockStream
	}
	return l.streamData, l.streamErr
}

func newTestRunner(rt runtime.Runtime) *CommandRunner {
	return NewCommandRunner(rt, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

func logsCmd(requestID string, follow bool) *agentpb.AgentCommand {
	return &agentpb.AgentCommand{
		RequestId: requestID,
		Command: &agentpb.AgentCommand_Logs{Logs: &agentpb.LogsRequest{
			ContainerId: "ctr", Lines: 100, Follow: follow,
		}},
	}
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestCommandRunner_SnapshotReturnsLinesAndTerminates(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(&logRuntime{fetch: []string{"line-1", "line-2"}})

	r.Handle(context.Background(), out, logsCmd("req-1", false))

	waitFor(t, func() bool { return len(out.snapshot()) == 1 }, "no result produced")
	res := out.snapshot()[0]
	assert.Equal(t, "req-1", res.GetRequestId())
	assert.True(t, res.GetLast(), "a snapshot answer must be terminal")
	assert.Empty(t, res.GetErrorCode())
	assert.Equal(t, []string{"line-1", "line-2"}, res.GetLogs().GetLines())
}

func TestCommandRunner_SnapshotReportsRuntimeError(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(&logRuntime{fetchErr: errors.New("no such container")})

	r.Handle(context.Background(), out, logsCmd("req-err", false))

	waitFor(t, func() bool { return len(out.snapshot()) == 1 }, "no result produced")
	res := out.snapshot()[0]
	assert.Equal(t, "logs_unavailable", res.GetErrorCode())
	assert.Contains(t, res.GetErrorMessage(), "no such container")
	assert.True(t, res.GetLast())
}

func TestCommandRunner_NilRuntimeIsReportedNotSwallowed(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(nil)

	r.Handle(context.Background(), out, logsCmd("req-nort", false))

	waitFor(t, func() bool { return len(out.snapshot()) == 1 }, "no result produced")
	assert.Equal(t, "runtime_unavailable", out.snapshot()[0].GetErrorCode())
}

func TestCommandRunner_MissingContainerIDRejected(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(&logRuntime{})

	r.Handle(context.Background(), out, &agentpb.AgentCommand{
		RequestId: "req-empty",
		Command:   &agentpb.AgentCommand_Logs{Logs: &agentpb.LogsRequest{}},
	})

	waitFor(t, func() bool { return len(out.snapshot()) == 1 }, "no result produced")
	assert.Equal(t, "invalid_request", out.snapshot()[0].GetErrorCode())
}

func TestCommandRunner_FollowStreamsLinesThenTerminates(t *testing.T) {
	reader := &trackingReader{Reader: strings.NewReader("alpha\nbeta\ngamma\n")}
	out := &captureSender{}
	r := newTestRunner(&logRuntime{streamData: reader})

	r.Handle(context.Background(), out, logsCmd("req-follow", true))

	waitFor(t, func() bool {
		got := out.snapshot()
		return len(got) > 0 && got[len(got)-1].GetLast()
	}, "follow never sent a terminal result")

	assert.Equal(t, []string{"alpha", "beta", "gamma"}, out.lines())
	assert.True(t, reader.closed.Load(), "the runtime reader must be closed when the follow ends")
}

func TestCommandRunner_CancelStopsFollowAndClosesReader(t *testing.T) {
	// A reader that never reaches EOF, standing in for a live container.
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	reader := &trackingReader{Reader: pr}

	out := &captureSender{}
	r := newTestRunner(&logRuntime{streamData: reader})

	r.Handle(context.Background(), out, logsCmd("req-cancel", true))
	waitFor(t, func() bool { return r.inFlight() == 1 }, "follow never registered")

	r.Handle(context.Background(), out, &agentpb.AgentCommand{
		RequestId: "req-cancel",
		Command:   &agentpb.AgentCommand_Cancel{Cancel: &agentpb.CancelRequest{}},
	})

	waitFor(t, func() bool { return reader.closed.Load() }, "cancel did not close the runtime reader")
	waitFor(t, func() bool { return r.inFlight() == 0 }, "cancelled request stayed registered")
}

func TestCommandRunner_CancelAllStopsEverything(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	reader := &trackingReader{Reader: pr}

	out := &captureSender{}
	r := newTestRunner(&logRuntime{streamData: reader})

	r.Handle(context.Background(), out, logsCmd("req-all", true))
	waitFor(t, func() bool { return r.inFlight() == 1 }, "follow never registered")

	r.CancelAll()

	waitFor(t, func() bool { return reader.closed.Load() }, "CancelAll did not close the reader")
}

func TestCommandRunner_RefusesFollowsBeyondCap(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	out := &captureSender{}
	r := newTestRunner(&logRuntime{streamData: &trackingReader{Reader: pr}})

	for i := 0; i < maxConcurrentFollows; i++ {
		r.Handle(context.Background(), out, logsCmd("req-"+string(rune('a'+i)), true))
	}
	waitFor(t, func() bool { return r.inFlight() == maxConcurrentFollows }, "follows never registered")

	r.Handle(context.Background(), out, logsCmd("req-over", true))

	waitFor(t, func() bool {
		for _, res := range out.snapshot() {
			if res.GetRequestId() == "req-over" {
				return res.GetErrorCode() == "too_many_follows"
			}
		}
		return false
	}, "the follow beyond the cap was not refused")

	r.CancelAll()
}

func TestCommandRunner_UnknownCommandIsReported(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(&logRuntime{})

	r.Handle(context.Background(), out, &agentpb.AgentCommand{RequestId: "req-unknown"})

	waitFor(t, func() bool { return len(out.snapshot()) == 1 }, "no result produced")
	assert.Equal(t, "unsupported_command", out.snapshot()[0].GetErrorCode())
}

func TestCommandRunner_EmptyRequestIDIgnored(t *testing.T) {
	out := &captureSender{}
	r := newTestRunner(&logRuntime{})

	r.Handle(context.Background(), out, logsCmd("", false))

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, out.snapshot(), "a command with no request id cannot be answered")
}

func TestClampLogLines(t *testing.T) {
	assert.Equal(t, 100, clampLogLines(0))
	assert.Equal(t, maxLogLines, clampLogLines(10_000))
	assert.Equal(t, 42, clampLogLines(42))
}

func TestCapabilities_AdvertisesLogs(t *testing.T) {
	require.Contains(t, Capabilities(), CapabilityLogs)
}
