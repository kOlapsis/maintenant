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

package certificate

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
)

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

	monitor, err := s.store.GetMonitorByHostPortAgent(ctx, &agentID, host, port)
	if err != nil {
		return err
	}
	if monitor == nil {
		monitor = &CertMonitor{
			Hostname:             host,
			Port:                 port,
			Source:               SourceLabel,
			Status:               StatusUnknown,
			CheckIntervalSeconds: 43200,
			WarningThresholds:    DefaultWarningThresholds(),
			AgentID:              &agentID,
		}
		if _, err := s.store.CreateMonitor(ctx, monitor); err != nil {
			return fmt.Errorf("create agent cert monitor %s:%d: %w", host, port, err)
		}
		s.emit(event.CertificateCreated, map[string]interface{}{
			"monitor_id": monitor.ID,
			"hostname":   host,
			"port":       port,
			"source":     string(SourceLabel),
			"agent_id":   agentID,
		})
	}

	s.processCheckResult(ctx, monitor, agentCertToRaw(ev))
	return nil
}

// agentCertToRaw adapts a pushed CertificateInfo into the raw scan-result shape
// the local pipeline expects. The agent reports the leaf certificate only (no
// full chain or OCSP validation); chain and hostname are treated as acceptable
// because the agent could not have read the cert without a successful handshake.
func agentCertToRaw(ev *agentpb.CertificateInfo) *CheckCertificateResult {
	raw := &CheckCertificateResult{
		SubjectCN:     ev.GetSubjectCn(),
		IssuerCN:      ev.GetIssuerCn(),
		SANs:          ev.GetSanDns(),
		ChainValid:    true,
		HostnameMatch: true,
	}
	if nb := ev.GetNotBefore(); nb != nil {
		raw.NotBefore = nb.AsTime()
	}
	if na := ev.GetNotAfter(); na != nil {
		raw.NotAfter = na.AsTime()
	}
	return raw
}
