// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
)

// CertPostureAdapter adapts the certificate service for posture scoring.
type CertPostureAdapter struct {
	CertSvc *certificate.Service
}

// CertificatesByContainer lists the monitors once and keeps those of the given containers.
func (a *CertPostureAdapter) CertificatesByContainer(ctx context.Context, containerExternalIDs []string) (map[string][]security.CertificateInfo, error) {
	wanted := idSet(containerExternalIDs)
	monitors, err := a.CertSvc.ListMonitors(ctx, certificate.ListCertificatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list cert monitors: %w", err)
	}

	result := make(map[string][]security.CertificateInfo)
	for _, m := range monitors {
		if !wanted[m.ExternalID] {
			continue
		}
		info := security.CertificateInfo{
			Status: string(m.Status),
		}
		cr, err := a.CertSvc.GetLatestCheckResult(ctx, m.ID)
		if err == nil && cr != nil {
			info.DaysRemaining = cr.DaysRemaining()
		}
		result[m.ExternalID] = append(result[m.ExternalID], info)
	}
	return result, nil
}

// CVEPostureAdapter adapts the update store for CVE scoring.
type CVEPostureAdapter struct {
	Store update.UpdateStore
}

func (a *CVEPostureAdapter) ListCVEsForContainer(ctx context.Context, containerExternalID string) ([]security.CVEInfo, error) {
	cves, err := a.Store.ListContainerCVEs(ctx, containerExternalID)
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

func (a *CVEEvaluationPostureAdapter) GetCVEEvaluation(ctx context.Context, containerExternalID string) (*security.CVEEvaluationInfo, error) {
	eval, err := a.Store.GetCVEEvaluation(ctx, containerExternalID)
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
func (a *UpdatePostureAdapter) UpdatesByContainer(ctx context.Context, containerExternalIDs []string) (map[string][]security.UpdateInfo, error) {
	wanted := idSet(containerExternalIDs)
	updates, err := a.Store.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list image updates: %w", err)
	}
	result := make(map[string][]security.UpdateInfo)
	for _, u := range updates {
		if !wanted[u.ContainerID] {
			continue
		}
		result[u.ContainerID] = append(result[u.ContainerID], security.UpdateInfo{
			UpdateType:  string(u.UpdateType),
			PublishedAt: u.PublishedAt,
		})
	}
	return result, nil
}

func idSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
