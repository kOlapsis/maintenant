// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package webhook

import (
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

// ValidEventTypes is the set of all valid event type values for webhook subscriptions.
var ValidEventTypes = map[string]bool{
	"*":                            true,
	event.ContainerStateChanged:    true,
	event.EndpointStatusChanged:    true,
	event.HeartbeatStatusChanged:   true,
	event.CertificateStatusChanged: true,
	event.AlertFired:               true,
	event.AlertResolved:            true,
}

// MaxConsecutiveFailures is the threshold at which a webhook is auto-disabled.
const MaxConsecutiveFailures = 10

// WebhookSubscription represents a registered webhook URL.
type WebhookSubscription struct {
	ID                 string     `json:"id"`
	UserID             string     `json:"user_id"`
	Name               string     `json:"name"`
	URL                string     `json:"url"`
	Secret             string     `json:"secret,omitempty"`
	EventTypes         []string   `json:"event_types"`
	IsActive           bool       `json:"is_active"`
	LastDeliveryStatus *string    `json:"last_delivery_status"`
	LastDeliveryAt     *time.Time `json:"last_delivery_at"`
	FailureCount       int        `json:"failure_count"`
	CreatedAt          time.Time  `json:"created_at"`
}

// WebhookEvent is the payload delivered to webhook URLs.
type WebhookEvent struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}
