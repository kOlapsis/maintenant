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
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/runtime"
)

const (
	endpointProbeInterval = 30 * time.Second
	certScanInterval      = 60 * time.Second
	certScanTimeout       = 10 * time.Second
)

// eventSender pushes an event on the stream; decoupled from *PushStream so the
// probe functions can be unit-tested with a capturing sender.
type eventSender func(*agentpb.AgentEvent) error

// runLabelProbers probes the endpoints (maintenant.endpoint.*) and TLS
// certificates (maintenant.tls.certificates) declared via labels on the agent's
// containers and pushes the results. The AGENT owns this probing because the
// targets live on its own host/network; the server provisions the monitors from
// the same labels but never dials them itself (FR-018a). Blocks until ctx done.
func runLabelProbers(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
	ld, ok := rt.(labeledDiscoverer)
	if !ok {
		logger.Debug("collector: runtime exposes no labelled discovery; endpoint/cert label probing disabled")
		<-ctx.Done()
		return nil
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return probeLoop(gCtx, endpointProbeInterval, func() error {
			return probeEndpointsOnce(gCtx, id.AgentID, ld, stream.Send, logger)
		})
	})
	g.Go(func() error {
		return probeLoop(gCtx, certScanInterval, func() error {
			return scanCertsOnce(gCtx, id.AgentID, ld, stream.Send, logger)
		})
	})
	return g.Wait()
}

// probeLoop runs fn immediately, then every interval until ctx is cancelled.
// A non-nil fn error (a failed stream send) propagates so the collector tears
// down and reconnects.
func probeLoop(ctx context.Context, interval time.Duration, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := fn(); err != nil {
				return err
			}
		}
	}
}

// probeEndpointsOnce discovers labelled endpoints across the agent's containers,
// probes each unique target (HTTP/TCP, reusing the server's checkers for parity)
// and pushes an EndpointEvent. The server attaches the result to the monitor it
// provisioned for the same target.
func probeEndpointsOnce(ctx context.Context, agentID string, ld labeledDiscoverer, send eventSender, logger *slog.Logger) error {
	results, err := ld.DiscoverAllWithLabels(ctx)
	if err != nil {
		logger.Warn("prober: endpoint discovery failed", "err", err)
		return nil
	}
	seen := make(map[string]bool)
	for _, res := range results {
		parsed, _ := endpoint.ParseEndpointLabels(res.Labels, logger)
		for _, p := range parsed {
			if seen[p.Target] {
				continue
			}
			seen[p.Target] = true

			ep := &endpoint.Endpoint{EndpointType: p.EndpointType, Target: p.Target, Config: p.Config}
			var result endpoint.CheckResult
			switch p.EndpointType {
			case endpoint.TypeHTTP:
				result = endpoint.CheckHTTP(ctx, ep, logger)
			case endpoint.TypeTCP:
				result = endpoint.CheckTCP(ctx, ep, logger)
			default:
				continue
			}
			if err := send(endpointEvent(agentID, p.Target, result)); err != nil {
				return fmt.Errorf("send endpoint event: %w", err)
			}
		}
	}
	return nil
}

// scanCertsOnce discovers labelled TLS targets, scans each unique host:port
// (reusing the server's TLS checker) and pushes a CertificateInfo. Failed scans
// carry no certificate to report — CertificateInfo has no error field — so they
// are skipped; the next scan retries.
func scanCertsOnce(ctx context.Context, agentID string, ld labeledDiscoverer, send eventSender, logger *slog.Logger) error {
	results, err := ld.DiscoverAllWithLabels(ctx)
	if err != nil {
		logger.Warn("prober: cert discovery failed", "err", err)
		return nil
	}
	seen := make(map[string]bool)
	for _, res := range results {
		for _, c := range certificate.ParseCertificateLabels(res.Labels) {
			key := c.Hostname + ":" + strconv.Itoa(c.Port)
			if seen[key] {
				continue
			}
			seen[key] = true

			scan := certificate.CheckCertificate(c.Hostname, c.Port, "", certScanTimeout)
			if scan.Error != "" {
				logger.Debug("prober: cert scan failed", "host", c.Hostname, "port", c.Port, "err", scan.Error)
				continue
			}
			if err := send(certEvent(agentID, c.Hostname, c.Port, scan)); err != nil {
				return fmt.Errorf("send certificate event: %w", err)
			}
		}
	}
	return nil
}

// endpointEvent builds an EndpointEvent from a probe result. The target is
// carried in url so the server can resolve the monitor by (agent_id, target).
func endpointEvent(agentID, target string, r endpoint.CheckResult) *agentpb.AgentEvent {
	status := agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN
	if r.Success {
		status = agentpb.EndpointStatus_ENDPOINT_STATUS_UP
	}
	var code uint32
	if r.HTTPStatus != nil && *r.HTTPStatus > 0 {
		code = uint32(*r.HTTPStatus) // #nosec G115 -- HTTP status code, small positive value
	}
	var latency uint64
	if r.ResponseTimeMs > 0 {
		latency = uint64(r.ResponseTimeMs)
	}
	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{
			Url:          target,
			Status:       status,
			StatusCode:   code,
			LatencyMs:    latency,
			ErrorMessage: r.ErrorMessage,
		}},
	}
}

// certEvent builds a CertificateInfo from a TLS scan result.
func certEvent(agentID, host string, port int, scan *certificate.CheckCertificateResult) *agentpb.AgentEvent {
	info := &agentpb.CertificateInfo{
		Host:      host,
		Port:      uint32(port), // #nosec G115 -- TCP port, bounded to 0-65535
		SubjectCn: scan.SubjectCN,
		IssuerCn:  scan.IssuerCN,
		SanDns:    scan.SANs,
	}
	if !scan.NotBefore.IsZero() {
		info.NotBefore = timestamppb.New(scan.NotBefore)
	}
	if !scan.NotAfter.IsZero() {
		info.NotAfter = timestamppb.New(scan.NotAfter)
	}
	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body:       &agentpb.AgentEvent_Certificate{Certificate: info},
	}
}
