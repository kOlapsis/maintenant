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

package certificate

import (
	"encoding/json"
	"strings"
	"time"
)

// CertStatus represents the current status of a certificate monitor.
type CertStatus string

const (
	StatusValid    CertStatus = "valid"
	StatusExpiring CertStatus = "expiring"
	StatusExpired  CertStatus = "expired"
	StatusError    CertStatus = "error"
	StatusUnknown  CertStatus = "unknown"
)

// CertSource represents how the certificate monitor was created.
type CertSource string

const (
	SourceAuto       CertSource = "auto"
	SourceStandalone CertSource = "standalone"
	SourceLabel      CertSource = "label"
)

// CertMonitor represents a monitored SSL/TLS certificate.
type CertMonitor struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	// ServerName, when non-empty, is sent as SNI during the check and the
	// certificate is validated against it instead of Hostname.
	ServerName           string     `json:"server_name,omitempty"`
	Source               CertSource `json:"source"`
	EndpointID           *string    `json:"endpoint_id,omitempty"`
	Status               CertStatus `json:"status"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
	WarningThresholds    []int      `json:"warning_thresholds"`
	LastAlertedThreshold *int       `json:"last_alerted_threshold,omitempty"`
	LastCheckAt          *time.Time `json:"last_check_at,omitempty"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
	ExternalID           string     `json:"external_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	AgentID              string     `json:"agent_id"`
}

// DefaultWarningThresholds returns the default expiration warning thresholds in days.
func DefaultWarningThresholds() []int {
	return []int{30, 14, 7, 3, 1}
}

// autoRenewableIssuers lists certificate issuers known to support automatic renewal.
var autoRenewableIssuers = []string{
	"let's encrypt",
	"zerossl",
	"buypass",
	"google trust services",
}

// IsAutoRenewable returns true if the certificate issuer is known to support
// automatic renewal (ACME-based CAs like Let's Encrypt).
func IsAutoRenewable(issuerOrg string) bool {
	lower := strings.ToLower(issuerOrg)
	for _, issuer := range autoRenewableIssuers {
		if strings.Contains(lower, issuer) {
			return true
		}
	}
	return false
}

// ExpiringSeverity determines alert severity for an expiring certificate based
// on days remaining and whether the issuer supports automatic renewal.
// Auto-renewable certs get lower severity because renewal is expected.
func ExpiringSeverity(daysRemaining int, issuerOrg string) string {
	if IsAutoRenewable(issuerOrg) {
		switch {
		case daysRemaining <= 3:
			return "critical"
		case daysRemaining <= 7:
			return "warning"
		default:
			return "info"
		}
	}
	switch {
	case daysRemaining <= 7:
		return "critical"
	case daysRemaining <= 14:
		return "warning"
	default:
		return "info"
	}
}

// WarningThresholdsJSON returns the JSON-encoded warning thresholds.
func (m *CertMonitor) WarningThresholdsJSON() string {
	data, _ := json.Marshal(m.WarningThresholds)
	return string(data)
}

// CertCheckResult represents a single certificate check execution.
type CertCheckResult struct {
	ID                 string     `json:"id"`
	MonitorID          string     `json:"monitor_id"`
	SubjectCN          string     `json:"subject_cn,omitempty"`
	IssuerCN           string     `json:"issuer_cn,omitempty"`
	IssuerOrg          string     `json:"issuer_org,omitempty"`
	SANs               []string   `json:"sans,omitempty"`
	SerialNumber       string     `json:"serial_number,omitempty"`
	SignatureAlgorithm string     `json:"signature_algorithm,omitempty"`
	NotBefore          *time.Time `json:"not_before,omitempty"`
	NotAfter           *time.Time `json:"not_after,omitempty"`
	ChainValid         *bool      `json:"chain_valid,omitempty"`
	ChainError         string     `json:"chain_error,omitempty"`
	HostnameMatch      *bool      `json:"hostname_match,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	CheckedAt          time.Time  `json:"checked_at"`
	OCSPStapled        bool       `json:"ocsp_stapled,omitempty"`
	OCSPStatus         string     `json:"ocsp_status,omitempty"`
	OCSPProducedAt     *time.Time `json:"ocsp_produced_at,omitempty"`
	OCSPNextUpdate     *time.Time `json:"ocsp_next_update,omitempty"`
	OCSPError          string     `json:"ocsp_error,omitempty"`
}

// DaysRemaining returns the number of days until the certificate expires.
// Returns -1 if NotAfter is nil.
func (r *CertCheckResult) DaysRemaining() int {
	if r.NotAfter == nil {
		return -1
	}
	d := time.Until(*r.NotAfter)
	return int(d.Hours() / 24)
}

// SANsJSON returns the JSON-encoded SANs.
func (r *CertCheckResult) SANsJSON() string {
	if len(r.SANs) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(r.SANs)
	return string(data)
}

// CertChainEntry represents an individual certificate in the presented chain.
type CertChainEntry struct {
	ID            string    `json:"id"`
	CheckResultID string    `json:"check_result_id"`
	Position      int       `json:"position"`
	SubjectCN     string    `json:"subject_cn"`
	IssuerCN      string    `json:"issuer_cn"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
}

// ListCertificatesOpts configures certificate monitor listing queries.
type ListCertificatesOpts struct {
	Status string
	Source string
	// AgentFilter filters by agent_id. Nil = no filter; "local" = agent_id IS NULL; UUID = specific agent.
	AgentFilter *string
}

// ListChecksOpts configures check result listing queries.
type ListChecksOpts struct {
	Limit  int
	Offset int
}

// CreateCertificateInput represents the input for creating a standalone certificate monitor.
type CreateCertificateInput struct {
	Hostname             string `json:"hostname"`
	Port                 int    `json:"port"`
	ServerName           string `json:"server_name"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
	WarningThresholds    []int  `json:"warning_thresholds"`
}

// UpdateCertificateInput represents the input for updating a certificate monitor.
type UpdateCertificateInput struct {
	CheckIntervalSeconds *int  `json:"check_interval_seconds,omitempty"`
	WarningThresholds    []int `json:"warning_thresholds,omitempty"`
}
