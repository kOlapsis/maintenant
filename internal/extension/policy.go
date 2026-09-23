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

package extension

import (
	"context"
	"log/slog"
	"slices"
	"time"
)

// Policy is the tier table the core queries to decide what an edition opens.
type Policy interface {
	Capabilities() []Capability
	MinEdition(c Capability) Edition
	Limit(e Edition, r Resource) int
	HistoryCap(e Edition) time.Duration
}

// LicenseStatusUpdateWindowEnded is the licence status of a build released after its update window and grace.
const LicenseStatusUpdateWindowEnded = "update_window_ended"

// LicenseState is the licence status reported by an EditionSource.
type LicenseState struct {
	Edition          Edition   `json:"edition"`
	Plan             string    `json:"plan,omitempty"`
	Features         []string  `json:"features,omitempty"`
	Status           string    `json:"status"`
	VerifiedAt       time.Time `json:"verified_at,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	Message          string    `json:"message,omitempty"`
	UpdatesUntil     time.Time `json:"updates_until,omitempty"`
	UpdateGraceUntil time.Time `json:"update_grace_until,omitempty"`
}

// EditionChangeCallback is invoked after the edition moves from prev to next.
type EditionChangeCallback func(ctx context.Context, prev, next Edition)

// EditionSource resolves the running edition from a licence.
type EditionSource interface {
	Edition() Edition
	State() *LicenseState
	RegisterEditionChangeCallback(cb EditionChangeCallback)
	Start(ctx context.Context)
	Stop()
}

// SourceConfig carries what an EditionSource needs to read and verify a licence.
type SourceConfig struct {
	LicenseKey   string
	PublicKeyB64 string
	DataDir      string
	Version      string
	BuildDate    string
	Logger       *slog.Logger
}

// SourceFactory builds an EditionSource, or returns nil when there is no licence to read.
type SourceFactory func(cfg SourceConfig) (EditionSource, error)

var (
	policy    Policy = CommunityPolicy{}
	newSource SourceFactory
)

// Register installs the tier table and the licence-backed edition source.
func Register(p Policy, f SourceFactory) {
	policy = p
	newSource = f
}

// NewEditionSource builds the registered edition source, or returns nil when none applies.
func NewEditionSource(cfg SourceConfig) (EditionSource, error) {
	if newSource == nil {
		return nil, nil
	}
	return newSource(cfg)
}

var communityCapabilities = []Capability{
	CapAlertRouting,
	CapSwarmDashboard,
	CapK8sCluster,
	CapResourceHistory,
}

var communityLimits = map[Resource]int{
	ResourceEndpoints:        10,
	ResourceHeartbeats:       5,
	ResourceCertificates:     5,
	ResourceStatusComponents: 3,
	ResourceAgentHosts:       0,
}

const communityHistoryCap = 7 * 24 * time.Hour

// CommunityPolicy is the tier table of the Community edition, the one the core applies when nothing is registered.
type CommunityPolicy struct{}

// Capabilities lists the capabilities Community opens.
func (CommunityPolicy) Capabilities() []Capability {
	return slices.Clone(communityCapabilities)
}

// MinEdition returns Community for a Community capability and Pro for any other.
func (CommunityPolicy) MinEdition(c Capability) Edition {
	if slices.Contains(communityCapabilities, c) {
		return Community
	}
	return Pro
}

// Limit returns the Community cap on r, whatever the edition.
func (CommunityPolicy) Limit(_ Edition, r Resource) int {
	return communityLimits[r]
}

// HistoryCap returns the Community history cap, whatever the edition.
func (CommunityPolicy) HistoryCap(_ Edition) time.Duration {
	return communityHistoryCap
}
