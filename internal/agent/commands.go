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
	"bufio"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/runtime"
)

// CapabilityLogs mirrors agentserver.CapabilityLogs. Declared here rather than
// imported: the agent must not depend on the server package.
const CapabilityLogs = "logs"

const (
	// maxLogLines bounds a tail request, matching the runtime's own ceiling.
	maxLogLines = 500
	// maxConcurrentFollows caps live log streams, each of which holds a runtime
	// connection open for as long as a viewer watches it.
	maxConcurrentFollows = 4
	// followChunkLines and followFlushInterval trade latency against message
	// count: whichever comes first ends the current chunk.
	followChunkLines    = 100
	followFlushInterval = 250 * time.Millisecond
)

// Capabilities lists the command families this build understands. Sent at auth
// so the server can reject unsupported requests immediately instead of timing out.
func Capabilities() []string {
	return []string{CapabilityLogs}
}

// resultSender is the subset of PushStream the runner needs, kept narrow so
// tests can drive the runner without a gRPC stream.
type resultSender interface {
	SendResult(res *agentpb.CommandResult) error
}

// CommandRunner executes server-issued commands against the local runtime.
// One instance is shared across reconnections; in-flight work is keyed by the
// server-minted request id so a Cancel can reach it.
type CommandRunner struct {
	rt     runtime.Runtime
	logger *slog.Logger

	mu      sync.Mutex
	running map[string]context.CancelFunc
	follows int
}

// NewCommandRunner builds a runner over rt. rt may be nil, in which case every
// command is answered with an error rather than silently dropped.
func NewCommandRunner(rt runtime.Runtime, logger *slog.Logger) *CommandRunner {
	return &CommandRunner{
		rt:      rt,
		logger:  logger,
		running: make(map[string]context.CancelFunc),
	}
}

// Handle dispatches cmd, returning immediately: work runs in its own goroutine
// so the stream's receive loop is never blocked by a log tail.
func (r *CommandRunner) Handle(ctx context.Context, out resultSender, cmd *agentpb.AgentCommand) {
	requestID := cmd.GetRequestId()
	if requestID == "" {
		return
	}

	switch body := cmd.GetCommand().(type) {
	case *agentpb.AgentCommand_Cancel:
		r.cancel(requestID)
	case *agentpb.AgentCommand_Logs:
		go r.runLogs(ctx, out, requestID, body.Logs)
	default:
		r.fail(out, requestID, "unsupported_command", "this agent does not understand that command")
	}
}

// cancel aborts the in-flight request, if it is still running.
func (r *CommandRunner) cancel(requestID string) {
	r.mu.Lock()
	cancel, ok := r.running[requestID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
}

// inFlight reports how many commands are currently running.
func (r *CommandRunner) inFlight() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running)
}

// CancelAll aborts every in-flight command, used when the stream goes away.
func (r *CommandRunner) CancelAll() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.running))
	for _, c := range r.running {
		cancels = append(cancels, c)
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// track registers a cancellable request, refusing it when the follow budget is
// exhausted. The returned release must be called when the request completes.
func (r *CommandRunner) track(ctx context.Context, requestID string, follow bool) (context.Context, func(), bool) {
	r.mu.Lock()
	if follow && r.follows >= maxConcurrentFollows {
		r.mu.Unlock()
		return nil, nil, false
	}
	reqCtx, cancel := context.WithCancel(ctx)
	r.running[requestID] = cancel
	if follow {
		r.follows++
	}
	r.mu.Unlock()

	return reqCtx, func() {
		r.mu.Lock()
		delete(r.running, requestID)
		if follow {
			r.follows--
		}
		r.mu.Unlock()
		cancel()
	}, true
}

func (r *CommandRunner) runLogs(ctx context.Context, out resultSender, requestID string, req *agentpb.LogsRequest) {
	if r.rt == nil {
		r.fail(out, requestID, "runtime_unavailable", "no container runtime on this agent")
		return
	}
	containerID := req.GetContainerId()
	if containerID == "" {
		r.fail(out, requestID, "invalid_request", "container_id is required")
		return
	}

	follow := req.GetFollow()
	reqCtx, release, ok := r.track(ctx, requestID, follow)
	if !ok {
		r.fail(out, requestID, "too_many_follows", "this agent is already streaming its maximum number of log tails")
		return
	}
	defer release()

	lines := clampLogLines(req.GetLines())
	if follow {
		r.followLogs(reqCtx, out, requestID, containerID, lines, req.GetTimestamps())
		return
	}

	logLines, err := r.rt.FetchLogs(reqCtx, containerID, lines, req.GetTimestamps())
	if err != nil {
		r.fail(out, requestID, "logs_unavailable", err.Error())
		return
	}
	r.send(out, &agentpb.CommandResult{
		RequestId: requestID,
		Last:      true,
		Result:    &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{Lines: logLines}},
	})
}

// followLogs streams the container's output until the request is cancelled or
// the container stops.
func (r *CommandRunner) followLogs(ctx context.Context, out resultSender, requestID, containerID string, lines int, timestamps bool) {
	reader, err := r.rt.StreamLogs(ctx, containerID, lines, timestamps)
	if err != nil {
		r.fail(out, requestID, "logs_unavailable", err.Error())
		return
	}
	// Closing the reader is what releases the runtime connection; without it every
	// closed viewer would leak one.
	defer func() { _ = reader.Close() }()

	lineCh := make(chan string, followChunkLines)
	go func() {
		defer close(lineCh)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(followFlushInterval)
	defer ticker.Stop()

	buf := make([]string, 0, followChunkLines)
	flush := func(last bool) bool {
		if len(buf) == 0 && !last {
			return true
		}
		res := &agentpb.CommandResult{
			RequestId: requestID,
			Last:      last,
			Result:    &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{Lines: buf}},
		}
		buf = make([]string, 0, followChunkLines)
		return r.send(out, res)
	}

	for {
		select {
		case <-ctx.Done():
			// Cancelled by the server or the stream died: no terminal result is
			// owed, the requester is already gone.
			return

		case line, ok := <-lineCh:
			if !ok {
				flush(true)
				return
			}
			buf = append(buf, line)
			if len(buf) >= followChunkLines && !flush(false) {
				return
			}

		case <-ticker.C:
			if !flush(false) {
				return
			}
		}
	}
}

// send delivers res, reporting whether the stream is still usable.
func (r *CommandRunner) send(out resultSender, res *agentpb.CommandResult) bool {
	if err := out.SendResult(res); err != nil {
		r.logger.Debug("agent: send command result failed", "request_id", res.GetRequestId(), "err", err)
		return false
	}
	return true
}

func (r *CommandRunner) fail(out resultSender, requestID, code, message string) {
	r.send(out, &agentpb.CommandResult{
		RequestId:    requestID,
		ErrorCode:    code,
		ErrorMessage: message,
		Last:         true,
	})
}

// clampLogLines narrows the wire's uint32 to a sane int tail length.
func clampLogLines(n uint32) int {
	if n == 0 {
		return 100
	}
	if n > maxLogLines {
		return maxLogLines
	}
	return int(n)
}
