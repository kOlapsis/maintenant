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

package endpoint

import (
	"crypto/x509"
	"encoding/json"
	"time"
)

// Duration is a time.Duration that marshals to/from a human-readable string (e.g., "30s").
type Duration time.Duration

func (d Duration) String() string { return time.Duration(d).String() }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// EndpointSource represents the origin of an endpoint definition.
type EndpointSource string

const (
	SourceLabel      EndpointSource = "label"
	SourceStandalone EndpointSource = "standalone"
)

// EndpointType represents the protocol type of a monitored endpoint.
type EndpointType string

const (
	TypeHTTP EndpointType = "http"
	TypeTCP  EndpointType = "tcp"
)

// EndpointStatus represents the current check status.
type EndpointStatus string

const (
	StatusUp      EndpointStatus = "up"
	StatusDown    EndpointStatus = "down"
	StatusUnknown EndpointStatus = "unknown"
)

// AlertState represents the current alert state of an endpoint.
type AlertState string

const (
	AlertNormal   AlertState = "normal"
	AlertAlerting AlertState = "alerting"
)

// Endpoint represents a monitored HTTP or TCP target.
type Endpoint struct {
	ID                   int64          `json:"id"`
	ContainerName        string         `json:"container_name"`
	LabelKey             string         `json:"label_key"`
	ExternalID           string         `json:"external_id"`
	EndpointType         EndpointType   `json:"endpoint_type"`
	Target               string         `json:"target"`
	Status               EndpointStatus `json:"status"`
	AlertState           AlertState     `json:"alert_state"`
	ConsecutiveFailures  int            `json:"consecutive_failures"`
	ConsecutiveSuccesses int            `json:"consecutive_successes"`
	LastCheckAt          *time.Time     `json:"last_check_at,omitempty"`
	LastResponseTimeMs   *int64         `json:"last_response_time_ms,omitempty"`
	LastHTTPStatus       *int           `json:"last_http_status,omitempty"`
	LastError            string         `json:"last_error,omitempty"`
	Config               EndpointConfig `json:"config"`
	Active               bool           `json:"active"`
	FirstSeenAt          time.Time      `json:"first_seen_at"`
	LastSeenAt           time.Time      `json:"last_seen_at"`
	OrchestrationGroup   string         `json:"orchestration_group,omitempty"`
	OrchestrationUnit    string         `json:"orchestration_unit,omitempty"`
	Source               EndpointSource `json:"source"`
	Name                 string         `json:"name,omitempty"`
	AgentID              *string        `json:"agent_id,omitempty"`
}

// ConfigJSON returns the JSON-encoded configuration.
func (e *Endpoint) ConfigJSON() string {
	data, _ := json.Marshal(e.Config)
	return string(data)
}

// CheckResult represents a single probe result for an endpoint.
type CheckResult struct {
	ID                  int64               `json:"id"`
	EndpointID          int64               `json:"endpoint_id"`
	Success             bool                `json:"success"`
	ResponseTimeMs      int64               `json:"response_time_ms"`
	HTTPStatus          *int                `json:"http_status,omitempty"`
	ErrorMessage        string              `json:"error_message,omitempty"`
	Timestamp           time.Time           `json:"timestamp"`
	TLSPeerCertificates []*x509.Certificate `json:"-"`
	TLSOCSPResponse     []byte              `json:"-"`
	AgentID             *string             `json:"agent_id,omitempty"`
}

// EndpointConfig holds the configuration parameters for endpoint checks.
type EndpointConfig struct {
	Interval          Duration          `json:"interval"`
	Timeout           Duration          `json:"timeout"`
	FailureThreshold  int               `json:"failure_threshold"`
	RecoveryThreshold int               `json:"recovery_threshold"`
	Method            string            `json:"method,omitempty"`
	ExpectedStatus    string            `json:"expected_status,omitempty"`
	TLSVerify         bool              `json:"tls_verify"`
	Headers           map[string]string `json:"headers,omitempty"`
	MaxRedirects      int               `json:"max_redirects,omitempty"`
}

// DefaultConfig returns an EndpointConfig with sensible defaults.
func DefaultConfig() EndpointConfig {
	return EndpointConfig{
		Interval:          Duration(30 * time.Second),
		Timeout:           Duration(10 * time.Second),
		FailureThreshold:  3,
		RecoveryThreshold: 2,
		Method:            "GET",
		ExpectedStatus:    "2xx",
		TLSVerify:         true,
		Headers:           make(map[string]string),
		MaxRedirects:      5,
	}
}

// ListEndpointsOpts configures endpoint listing queries.
type ListEndpointsOpts struct {
	Status             string
	ContainerName      string
	OrchestrationGroup string
	EndpointType       string
	Source             string
	IncludeInactive    bool
	AgentFilter *string
}

// ListChecksOpts configures check result listing queries.
type ListChecksOpts struct {
	Limit  int
	Offset int
	Since  *time.Time
}
