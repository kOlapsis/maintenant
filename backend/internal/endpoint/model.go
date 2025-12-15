package endpoint

import (
	"crypto/x509"
	"encoding/json"
	"time"
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

// Endpoint represents a monitored HTTP or TCP target associated with a container.
type Endpoint struct {
	ID                    int64          `json:"id"`
	ContainerName         string         `json:"container_name"`
	LabelKey              string         `json:"label_key"`
	ExternalID            string         `json:"external_id"`
	EndpointType          EndpointType   `json:"endpoint_type"`
	Target                string         `json:"target"`
	Status                EndpointStatus `json:"status"`
	AlertState            AlertState     `json:"alert_state"`
	ConsecutiveFailures   int            `json:"consecutive_failures"`
	ConsecutiveSuccesses  int            `json:"consecutive_successes"`
	LastCheckAt           *time.Time     `json:"last_check_at,omitempty"`
	LastResponseTimeMs    *int64         `json:"last_response_time_ms,omitempty"`
	LastHTTPStatus        *int           `json:"last_http_status,omitempty"`
	LastError             string         `json:"last_error,omitempty"`
	Config                EndpointConfig `json:"config"`
	Active                bool           `json:"active"`
	FirstSeenAt           time.Time      `json:"first_seen_at"`
	LastSeenAt            time.Time      `json:"last_seen_at"`
	OrchestrationGroup    string         `json:"orchestration_group,omitempty"`
	OrchestrationUnit     string         `json:"orchestration_unit,omitempty"`
}

// ConfigJSON returns the JSON-encoded configuration.
func (e *Endpoint) ConfigJSON() string {
	data, _ := json.Marshal(e.Config)
	return string(data)
}

// CheckResult represents a single probe result for an endpoint.
type CheckResult struct {
	ID                  int64                `json:"id"`
	EndpointID          int64                `json:"endpoint_id"`
	Success             bool                 `json:"success"`
	ResponseTimeMs      int64                `json:"response_time_ms"`
	HTTPStatus          *int                 `json:"http_status,omitempty"`
	ErrorMessage        string               `json:"error_message,omitempty"`
	Timestamp           time.Time            `json:"timestamp"`
	TLSPeerCertificates []*x509.Certificate  `json:"-"`
}

// EndpointConfig holds the configuration parameters for endpoint checks.
type EndpointConfig struct {
	Interval          time.Duration     `json:"interval"`
	Timeout           time.Duration     `json:"timeout"`
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
		Interval:          30 * time.Second,
		Timeout:           10 * time.Second,
		FailureThreshold:  3,
		RecoveryThreshold: 2,
		Method:            "GET",
		ExpectedStatus:    "2xx",
		TLSVerify:         true,
		Headers:           make(map[string]string),
		MaxRedirects:      5,
	}
}

// MarshalJSON implements custom JSON marshaling for EndpointConfig to use string durations.
func (c EndpointConfig) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Interval          string            `json:"interval"`
		Timeout           string            `json:"timeout"`
		FailureThreshold  int               `json:"failure_threshold"`
		RecoveryThreshold int               `json:"recovery_threshold"`
		Method            string            `json:"method,omitempty"`
		ExpectedStatus    string            `json:"expected_status,omitempty"`
		TLSVerify         bool              `json:"tls_verify"`
		Headers           map[string]string `json:"headers,omitempty"`
		MaxRedirects      int               `json:"max_redirects,omitempty"`
	}
	return json.Marshal(Alias{
		Interval:          c.Interval.String(),
		Timeout:           c.Timeout.String(),
		FailureThreshold:  c.FailureThreshold,
		RecoveryThreshold: c.RecoveryThreshold,
		Method:            c.Method,
		ExpectedStatus:    c.ExpectedStatus,
		TLSVerify:         c.TLSVerify,
		Headers:           c.Headers,
		MaxRedirects:      c.MaxRedirects,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for EndpointConfig with string durations.
func (c *EndpointConfig) UnmarshalJSON(data []byte) error {
	type Alias struct {
		Interval          string            `json:"interval"`
		Timeout           string            `json:"timeout"`
		FailureThreshold  int               `json:"failure_threshold"`
		RecoveryThreshold int               `json:"recovery_threshold"`
		Method            string            `json:"method,omitempty"`
		ExpectedStatus    string            `json:"expected_status,omitempty"`
		TLSVerify         bool              `json:"tls_verify"`
		Headers           map[string]string `json:"headers,omitempty"`
		MaxRedirects      int               `json:"max_redirects,omitempty"`
	}
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	if a.Interval != "" {
		d, err := time.ParseDuration(a.Interval)
		if err != nil {
			return err
		}
		c.Interval = d
	}
	if a.Timeout != "" {
		d, err := time.ParseDuration(a.Timeout)
		if err != nil {
			return err
		}
		c.Timeout = d
	}
	c.FailureThreshold = a.FailureThreshold
	c.RecoveryThreshold = a.RecoveryThreshold
	c.Method = a.Method
	c.ExpectedStatus = a.ExpectedStatus
	c.TLSVerify = a.TLSVerify
	c.Headers = a.Headers
	c.MaxRedirects = a.MaxRedirects
	return nil
}

// ListEndpointsOpts configures endpoint listing queries.
type ListEndpointsOpts struct {
	Status         string
	ContainerName  string
	OrchestrationGroup string
	EndpointType   string
	IncludeInactive bool
}

// ListChecksOpts configures check result listing queries.
type ListChecksOpts struct {
	Limit  int
	Offset int
	Since  *time.Time
}
