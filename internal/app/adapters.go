// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

// CertPostureAdapter adapts the certificate service for posture scoring.
type CertPostureAdapter struct {
	CertSvc *certificate.Service
}

// CertificatesByContainer lists the monitors once and keeps those of the given containers.
func (a *CertPostureAdapter) CertificatesByContainer(ctx context.Context, containers []security.ContainerInfo) (map[string][]security.CertificateInfo, error) {
	wanted := containerIDs(containers)
	monitors, err := a.CertSvc.ListMonitors(ctx, certificate.ListCertificatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list cert monitors: %w", err)
	}

	result := make(map[string][]security.CertificateInfo)
	for _, m := range monitors {
		id, ok := wanted[hostedContainer{uid.Agent(m.AgentID), m.ExternalID}]
		if !ok {
			continue
		}
		info := security.CertificateInfo{
			Status: string(m.Status),
		}
		cr, err := a.CertSvc.GetLatestCheckResult(ctx, m.ID)
		if err == nil && cr != nil {
			info.DaysRemaining = cr.DaysRemaining()
		}
		result[id] = append(result[id], info)
	}
	return result, nil
}

// CVEPostureAdapter adapts the update store for CVE scoring.
type CVEPostureAdapter struct {
	Store update.UpdateStore
}

func (a *CVEPostureAdapter) ListCVEsForContainer(ctx context.Context, agentID, containerExternalID string) ([]security.CVEInfo, error) {
	cves, err := a.Store.ListContainerCVEs(ctx, agentID, containerExternalID)
	if err != nil {
		return nil, fmt.Errorf("list container cves: %w", err)
	}
	result := make([]security.CVEInfo, len(cves))
	for i, c := range cves {
		result[i] = security.CVEInfo{
			CVEID:    c.CVEID,
			Severity: string(c.Severity),
		}
	}
	return result, nil
}

// CVEEvaluationPostureAdapter adapts the update store for CVE evaluation state.
type CVEEvaluationPostureAdapter struct {
	Store update.UpdateStore
}

func (a *CVEEvaluationPostureAdapter) GetCVEEvaluation(ctx context.Context, agentID, containerExternalID string) (*security.CVEEvaluationInfo, error) {
	eval, err := a.Store.GetCVEEvaluation(ctx, agentID, containerExternalID)
	if err != nil {
		return nil, fmt.Errorf("get cve evaluation: %w", err)
	}
	if eval == nil {
		return nil, nil
	}
	return &security.CVEEvaluationInfo{Status: string(eval.Status)}, nil
}

// UpdatePostureAdapter adapts the update store for update/image-age scoring.
type UpdatePostureAdapter struct {
	Store update.UpdateStore
}

// UpdatesByContainer lists the pending updates once and keeps those of the given containers.
func (a *UpdatePostureAdapter) UpdatesByContainer(ctx context.Context, containers []security.ContainerInfo) (map[string][]security.UpdateInfo, error) {
	wanted := containerIDs(containers)
	updates, err := a.Store.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list image updates: %w", err)
	}
	result := make(map[string][]security.UpdateInfo)
	for _, u := range updates {
		id, ok := wanted[hostedContainer{uid.Agent(u.AgentID), u.ContainerID}]
		if !ok {
			continue
		}
		result[id] = append(result[id], security.UpdateInfo{
			UpdateType:  string(u.UpdateType),
			PublishedAt: u.PublishedAt,
		})
	}
	return result, nil
}

type hostedContainer struct{ agentID, externalID string }

func containerIDs(containers []security.ContainerInfo) map[hostedContainer]string {
	ids := make(map[hostedContainer]string, len(containers))
	for _, c := range containers {
		ids[hostedContainer{uid.Agent(c.AgentID), c.ExternalID}] = c.ID
	}
	return ids
}
