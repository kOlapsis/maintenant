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

func (a *CertPostureAdapter) ListCertificatesForContainer(ctx context.Context, containerExternalID string) ([]security.CertificateInfo, error) {
	monitors, err := a.CertSvc.ListMonitors(ctx, certificate.ListCertificatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list cert monitors: %w", err)
	}

	var result []security.CertificateInfo
	for _, m := range monitors {
		if m.ExternalID != containerExternalID {
			continue
		}
		info := security.CertificateInfo{
			Status: string(m.Status),
		}
		cr, err := a.CertSvc.GetLatestCheckResult(ctx, m.ID)
		if err == nil && cr != nil {
			info.DaysRemaining = cr.DaysRemaining()
		}
		result = append(result, info)
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

// UpdatePostureAdapter adapts the update store for update/image-age scoring.
type UpdatePostureAdapter struct {
	Store update.UpdateStore
}

func (a *UpdatePostureAdapter) ListUpdatesForContainer(ctx context.Context, containerExternalID string) ([]security.UpdateInfo, error) {
	updates, err := a.Store.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	if err != nil {
		return nil, fmt.Errorf("list image updates: %w", err)
	}
	var result []security.UpdateInfo
	for _, u := range updates {
		if u.ContainerID != containerExternalID {
			continue
		}
		result = append(result, security.UpdateInfo{
			UpdateType:  string(u.UpdateType),
			PublishedAt: u.PublishedAt,
		})
	}
	return result, nil
}
