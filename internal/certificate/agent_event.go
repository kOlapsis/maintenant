// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
)

var maxAgentLabelMonitors = container.MaxAgentRows

// HandleAgentEvent records a TLS certificate scan pushed by a remote agent.
//
// Push-create: the first scan an agent reports for a labelled host provisions
// the monitor, attributed to that agent. Agent monitors are never dialled by the
// local scheduler (the host lives on the agent's network, FR-018a) — their state
// is driven entirely by pushed scans. The result then flows through the same
// post-check pipeline as a local scan (status, alert evaluation, persistence).
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.CertificateInfo) error {
	host := ev.GetHost()
	port := int(ev.GetPort())
	if host == "" || port == 0 {
		return nil
	}

	monitor, err := s.store.GetMonitorByHostPortAgent(ctx, &agentID, host, port, "")
	if err != nil {
		return err
	}
	if monitor == nil {
		if err := s.reserveAgentLabelMonitor(ctx, agentID); err != nil {
			return err
		}
		monitor = &CertMonitor{
			Hostname:             host,
			Port:                 port,
			Source:               SourceLabel,
			Status:               StatusUnknown,
			CheckIntervalSeconds: 43200,
			WarningThresholds:    DefaultWarningThresholds(),
			AgentID:              agentID,
		}
		if _, err := s.store.CreateMonitor(ctx, monitor); err != nil {
			return fmt.Errorf("create agent cert monitor %s:%d: %w", host, port, err)
		}
		s.applyLimitLogged(ctx)
		s.emitCreated(ctx, monitor, map[string]interface{}{"source": string(SourceLabel), "agent_id": agentID})
	}
	if monitor.Limited {
		return nil
	}

	_, err = s.applyCheckResult(ctx, monitor, agentCertToRaw(ev), true)
	return err
}

func (s *Service) reserveAgentLabelMonitor(ctx context.Context, agentID string) error {
	return container.ReserveAgentRow(ctx, s, agentID, "certificate monitor", maxAgentLabelMonitors, s.logger, func(ctx context.Context) (int, error) {
		held, err := s.store.ListMonitors(ctx, ListCertificatesOpts{Source: string(SourceLabel), AgentFilter: &agentID, IncludeInactive: true})
		if err != nil {
			return 0, fmt.Errorf("count agent cert monitors: %w", err)
		}
		return len(held), nil
	})
}

// agentCertToRaw adapts a pushed CertificateInfo into a scan result, taking chain and hostname as valid when the agent does not report them.
func agentCertToRaw(ev *agentpb.CertificateInfo) *CheckCertificateResult {
	raw := &CheckCertificateResult{
		SubjectCN:     ev.GetSubjectCn(),
		IssuerCN:      ev.GetIssuerCn(),
		SANs:          ev.GetSanDns(),
		ChainValid:    ev.ChainValid == nil || ev.GetChainValid(),
		ChainError:    ev.GetChainError(),
		HostnameMatch: ev.HostnameMatch == nil || ev.GetHostnameMatch(),
	}
	if nb := ev.GetNotBefore(); nb != nil {
		raw.NotBefore = nb.AsTime()
	}
	if na := ev.GetNotAfter(); na != nil {
		raw.NotAfter = na.AsTime()
	}
	return raw
}
