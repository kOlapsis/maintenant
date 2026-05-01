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
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
)

// HandleAgentEvent records a TLS certificate scan result from a remote agent.
// If no monitor exists for the reported host:port, the event is silently
// dropped — monitor creation from agent events is a future capability.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.CertificateInfo) error {
	host := ev.GetHost()
	port := int(ev.GetPort())
	if host == "" || port == 0 {
		return nil
	}

	monitor, err := s.store.GetMonitorByHostPort(ctx, host, port)
	if err != nil || monitor == nil {
		return err
	}

	result := &CertCheckResult{
		MonitorID: monitor.ID,
		SubjectCN: ev.GetSubjectCn(),
		IssuerCN:  ev.GetIssuerCn(),
		SANs:      ev.GetSanDns(),
		CheckedAt: time.Now(),
	}
	if nb := ev.GetNotBefore(); nb != nil {
		t := nb.AsTime()
		result.NotBefore = &t
	}
	if na := ev.GetNotAfter(); na != nil {
		t := na.AsTime()
		result.NotAfter = &t
	}

	if _, err = s.store.InsertCheckResult(ctx, result); err != nil {
		return err
	}

	data := map[string]interface{}{
		"monitor_id": monitor.ID,
		"hostname":   monitor.Hostname,
		"status":     string(monitor.Status),
		"checked_at": result.CheckedAt.Format(time.RFC3339),
		"agent_id":   agentID,
	}
	if result.NotAfter != nil {
		data["not_after"] = result.NotAfter.Format(time.RFC3339)
		data["days_remaining"] = result.DaysRemaining()
	}
	s.emit(event.CertificateCheckCompleted, data)
	return nil
}
