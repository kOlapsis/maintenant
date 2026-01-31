package webhook

import "time"

// Valid event types for webhook subscriptions.
const (
	EventAll                     = "*"
	EventContainerStateChanged   = "container.state_changed"
	EventEndpointStatusChanged   = "endpoint.status_changed"
	EventHeartbeatStatusChanged  = "heartbeat.status_changed"
	EventCertificateStatusChanged = "certificate.status_changed"
	EventAlertFired              = "alert.fired"
	EventAlertResolved           = "alert.resolved"
)

// ValidEventTypes is the set of all valid event type values.
var ValidEventTypes = map[string]bool{
	EventAll:                       true,
	EventContainerStateChanged:     true,
	EventEndpointStatusChanged:     true,
	EventHeartbeatStatusChanged:    true,
	EventCertificateStatusChanged:  true,
	EventAlertFired:                true,
	EventAlertResolved:             true,
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
