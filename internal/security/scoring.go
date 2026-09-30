// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"fmt"
	"time"
)

// CertificateInfo holds certificate monitor status for scoring.
type CertificateInfo struct {
	Status        string // "valid", "expiring", "expired", "error"
	DaysRemaining int
}

// CVEInfo holds a single CVE for scoring.
type CVEInfo struct {
	CVEID    string
	Severity string // "critical", "high", "medium", "low"
}

// UpdateInfo holds update availability for scoring.
type UpdateInfo struct {
	UpdateType  string // "major", "minor", "patch", "digest_only"
	PublishedAt *time.Time
}

// CertificateReader provides the certificates of the given containers, keyed by external ID.
type CertificateReader interface {
	CertificatesByContainer(ctx context.Context, containerExternalIDs []string) (map[string][]CertificateInfo, error)
}

// CVEReader provides CVE data for a container.
type CVEReader interface {
	ListCVEsForContainer(ctx context.Context, containerExternalID string) ([]CVEInfo, error)
}

// CVEEvaluationInfo reports whether a container went through CVE analysis.
type CVEEvaluationInfo struct {
	Status string // "evaluated", "unsupported", "error"
}

// CVEEvaluationReader reports the CVE analysis state of a container.
type CVEEvaluationReader interface {
	GetCVEEvaluation(ctx context.Context, containerExternalID string) (*CVEEvaluationInfo, error)
}

// UpdateReader provides the pending updates of the given containers, keyed by external ID.
type UpdateReader interface {
	UpdatesByContainer(ctx context.Context, containerExternalIDs []string) (map[string][]UpdateInfo, error)
}

// AcknowledgmentStore persists risk acknowledgments.
type AcknowledgmentStore interface {
	InsertAcknowledgment(ctx context.Context, ack *RiskAcknowledgment) (string, error)
	DeleteAcknowledgment(ctx context.Context, id string) error
	ListAcknowledgments(ctx context.Context, containerExternalID string) ([]*RiskAcknowledgment, error)
	GetAcknowledgment(ctx context.Context, id string) (*RiskAcknowledgment, error)
	IsAcknowledged(ctx context.Context, containerExternalID, findingType, findingKey string) (bool, error)
}

// Category evaluation states, reported by CategoryScore.Evaluation.
const (
	EvaluationEvaluated    = "evaluated"
	EvaluationUnsupported  = "unsupported"
	EvaluationNotEvaluated = "not_evaluated"
	EvaluationError        = "error"
)

// PostureAlertCallback is called when the infrastructure score crosses a threshold.
type PostureAlertCallback func(score int, previousScore int, color string, isBreach bool)

// PostureEventCallback is called to emit SSE events for posture changes.
type PostureEventCallback func(eventType string, data any)

// InsightFindingKey returns the dedup key for an insight's finding.
func InsightFindingKey(i Insight) string {
	if port, ok := i.Details["port"]; ok {
		proto := "tcp"
		if p, ok := i.Details["protocol"]; ok {
			proto = fmt.Sprintf("%v", p)
		}
		return fmt.Sprintf("%v/%s", port, proto)
	}
	return ""
}

// ContainerInfo holds minimal container data for infrastructure scoring.
type ContainerInfo struct {
	ID         string
	ExternalID string
	Name       string
}

// InsightsReader provides the network-exposure insights of a container.
type InsightsReader interface {
	GetContainerInsights(containerID string) *ContainerInsights
}

// PostureScorer computes container and infrastructure security posture.
type PostureScorer interface {
	ScoreContainer(ctx context.Context, containerID, containerExternalID, containerName string) (*SecurityScore, error)
	ScoreContainers(ctx context.Context, containers []ContainerInfo) ([]*SecurityScore, error)
	ScoreInfrastructure(ctx context.Context, containers []ContainerInfo) (*InfrastructurePosture, error)
	InvalidateCache(containerID string)
	Threshold() int
	SetPostureAlertCallback(cb PostureAlertCallback)
	SetPostureEventCallback(cb PostureEventCallback)
}
