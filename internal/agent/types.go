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
	"errors"
	"time"
)

// Agent represents a remote agent registered with the server.
type Agent struct {
	AgentID         string
	PublicKey       []byte // raw 32 bytes Ed25519
	Hostname        string
	Label           string
	OSArch          string
	AgentVersion    string
	DetectedRuntime string // "docker"|"swarm"|"kubernetes"
	Status          string // "active"|"revoked"
	LastSeenAt      *time.Time
	CreatedAt       time.Time
	RevokedAt       *time.Time
	RevokedBy       *string
}

// EnrollmentToken represents a one-time token for enrolling an agent. The
// cleartext is deliberately absent: only its SHA-256 is persisted, so a copy of
// the database file yields nothing replayable. TokenPrefix is the leading,
// non-secret slice kept so a read path can still name the token it matched.
type EnrollmentToken struct {
	TokenID           string
	TokenHash         string
	TokenPrefix       string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	ConsumedAt        *time.Time
	ConsumedByAgentID *string
}

// Sentinel errors for agent store operations.
var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrTokenNotFound        = errors.New("enrollment token not found")
	ErrTokenAlreadyConsumed = errors.New("enrollment token already consumed")
	ErrTokenExpired         = errors.New("enrollment token expired")
	ErrLabelTooLong         = errors.New("label exceeds 64 characters")
	ErrAgentRevoked         = errors.New("agent is revoked")
	ErrBadSignature         = errors.New("invalid Ed25519 signature")
	ErrClockSkew            = errors.New("clock skew exceeds 300s")
	ErrAgentUnknown         = errors.New("agent unknown")
	ErrHostLimitReached     = errors.New("agent host limit reached")
)
