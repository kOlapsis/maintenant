// Copyright 2026 Benjamin Touchard (kOlapsis)
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
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/hoststat"
)

// hostOSInterval is the cadence at which the host operating system identity is
// re-read. A var (not const) so tests can shorten it; never mutated in production.
var hostOSInterval = time.Hour

// hostOSEvent builds the AgentEvent restating the host operating system identity.
func hostOSEvent(id *Identity, rel hoststat.OSRelease) *agentpb.AgentEvent {
	return &agentpb.AgentEvent{
		AgentId:    id.AgentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_HostOs{HostOs: &agentpb.HostOSMsg{
			Id:                rel.ID,
			VersionId:         rel.VersionID,
			PrettyName:        rel.PrettyName,
			Source:            hostOSSourceToProto(rel.Source),
			UnavailableReason: rel.UnavailableReason,
		}},
	}
}

func hostOSSourceToProto(source string) agentpb.HostOSSource {
	switch source {
	case hoststat.OSSourceHostFile:
		return agentpb.HostOSSource_HOST_OS_SOURCE_HOST_FILE
	case hoststat.OSSourceKubernetesNode:
		return agentpb.HostOSSource_HOST_OS_SOURCE_KUBERNETES_NODE
	default:
		return agentpb.HostOSSource_HOST_OS_SOURCE_UNSPECIFIED
	}
}

// streamHostOS reports the host operating system identity at once, then re-reads
// it hourly and reports it again only when it changed.
func streamHostOS(ctx context.Context, id *Identity, read func() hoststat.OSRelease, spool *Spool, logger *slog.Logger) error {
	send := func(rel hoststat.OSRelease) {
		if err := spool.Send(hostOSEvent(id, rel)); err != nil {
			logger.Debug("collector: host os identity not sent", "error", err)
		}
	}

	last := read()
	send(last)

	ticker := time.NewTicker(hostOSInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			rel := read()
			if rel == last {
				continue
			}
			last = rel
			send(rel)
		}
	}
}
