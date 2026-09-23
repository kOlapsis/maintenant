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
package agentproto

import (
	"errors"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

// CapabilityLogs is advertised by agents able to serve container logs on demand.
const CapabilityLogs = "logs"

// Errors returned by the command path, mapped to HTTP status by the API layer.
var (
	ErrAgentNotConnected = errors.New("agent not connected")
	ErrAgentCannotServe  = errors.New("agent does not support this command")
	ErrTooManyRequests   = errors.New("too many in-flight commands for this agent")
)

// logsTailDefault and logsTailMax bound a tail request; the agent clamps too, but
// bounding here keeps the uint32 conversion provably safe.
const (
	logsTailDefault = 100
	logsTailMax     = 500
)

// LogsCommand builds a logs command with a fresh request id. Exported because the
// SSE follow path drives SendCommand directly to stream chunks as they arrive.
func LogsCommand(externalID string, lines int, timestamps, follow bool) *agentpb.AgentCommand {
	return &agentpb.AgentCommand{
		RequestId: uid.New(),
		Command: &agentpb.AgentCommand_Logs{Logs: &agentpb.LogsRequest{
			ContainerId: externalID,
			Lines:       clampTail(lines),
			Timestamps:  timestamps,
			Follow:      follow,
		}},
	}
}

// clampTail narrows a caller-supplied tail length to the wire's uint32. Written
// as early returns so both constant bounds directly guard the conversion: with
// the checks written as reassignments instead, static analysis cannot see that
// the converted value is already in range.
func clampTail(lines int) uint32 {
	if lines <= 0 {
		return logsTailDefault
	}
	if lines >= logsTailMax {
		return logsTailMax
	}
	return uint32(lines)
}

// SpoolState is what an agent last said about its outbound queue.
type SpoolState struct {
	Queued              int64
	Draining            bool
	DroppedSinceConnect int64
	ReportedAt          time.Time
}
