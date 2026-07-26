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

package v1

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/uid"
)

// remoteLogsTimeout bounds a one-shot tail from a remote agent. A follow is not
// subject to it: its lifetime is the HTTP request's.
const remoteLogsTimeout = 15 * time.Second

// AgentLogRequester issues log commands to a connected agent and reports whether
// it can serve them. Satisfied by *agentserver.Sessions.
type AgentLogRequester interface {
	IsConnected(agentID string) bool
	HasCapability(agentID, capability string) bool
	FetchLogs(ctx context.Context, agentID, externalID string, lines int, timestamps bool) ([]string, error)
	SendCommand(ctx context.Context, agentID, capability string, cmd *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error)
}

// isRemoteAgent reports whether the container lives on a remote agent's host
// rather than the server's own runtime.
func isRemoteAgent(agentID string) bool {
	return agentID != "" && agentID != uid.LocalAgent
}

// resolveAgentLabel returns the name an operator would recognise for an agent,
// preferring its label, then its hostname, and falling back to the raw id so a
// message is never left with a dangling reference.
func resolveAgentLabel(ctx context.Context, dir AgentDirectory, agentID string) string {
	if dir == nil {
		return agentID
	}
	names, err := dir.AgentNames(ctx)
	if err != nil {
		return agentID
	}
	n, ok := names[agentID]
	if !ok {
		return agentID
	}
	if n.Label != "" {
		return n.Label
	}
	if n.Hostname != "" {
		return n.Hostname
	}
	return agentID
}

// writeRemoteLogsError turns a command-dispatch failure into the most precise
// status we can give the operator. The old blanket "Cannot retrieve logs from
// Docker" was actively misleading for remote containers.
func writeRemoteLogsError(w http.ResponseWriter, agentLabel string, err error) {
	switch {
	case errors.Is(err, agentserver.ErrAgentNotConnected):
		WriteError(w, http.StatusServiceUnavailable, "AGENT_OFFLINE",
			"Agent "+agentLabel+" is offline — its container logs are unavailable until it reconnects.")
	case errors.Is(err, agentserver.ErrAgentCannotServe):
		WriteError(w, http.StatusNotImplemented, "AGENT_TOO_OLD",
			"Agent "+agentLabel+" runs a version that cannot serve logs remotely. Upgrade the agent to enable this.")
	case errors.Is(err, agentserver.ErrTooManyRequests):
		WriteError(w, http.StatusTooManyRequests, "LOGS_BUSY",
			"Agent "+agentLabel+" is already serving its maximum number of log requests. Close a log view and retry.")
	case errors.Is(err, context.DeadlineExceeded):
		WriteError(w, http.StatusGatewayTimeout, "LOGS_TIMEOUT",
			"Agent "+agentLabel+" did not answer in time.")
	default:
		WriteError(w, http.StatusBadGateway, "LOGS_UNAVAILABLE",
			"Could not retrieve logs from agent "+agentLabel+".")
	}
}

// fetchRemoteLogs performs a one-shot tail against a remote agent under a bounded
// deadline, so a silent agent fails the request instead of hanging the client.
func fetchRemoteLogs(ctx context.Context, req AgentLogRequester, agentID, externalID string, lines int, timestamps bool) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteLogsTimeout)
	defer cancel()
	return req.FetchLogs(ctx, agentID, externalID, lines, timestamps)
}
