package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ocsp"
)

// CheckCertificateResult holds the raw result of a TLS certificate check.
type CheckCertificateResult struct {
	SubjectCN          string
	IssuerCN           string
	IssuerOrg          string
	SANs               []string
	SerialNumber       string
	SignatureAlgorithm string
	NotBefore          time.Time
	NotAfter           time.Time
	ChainValid         bool
	ChainError         string
	HostnameMatch      bool
	Chain              []ChainCert
	Error              string

	OCSPStapled    bool
	OCSPStatus     string
	OCSPProducedAt *time.Time
	OCSPNextUpdate *time.Time
	OCSPError      string
}

// ChainCert represents a certificate in the chain.
type ChainCert struct {
	SubjectCN string
	IssuerCN  string
	NotBefore time.Time
	NotAfter  time.Time
}

// CheckCertificate performs a TLS handshake to the given hostname:port and extracts
// certificate details including chain validation. A non-empty serverName is sent
// as SNI and used for chain validation and hostname matching instead of hostname,
// so a failover proxy can be checked for the certificate it serves a given vhost.
func CheckCertificate(hostname string, port int, serverName string, timeout time.Duration) *CheckCertificateResult {
	addr := fmt.Sprintf("%s:%d", hostname, port)
	validationName := serverName
	if validationName == "" {
		validationName = hostname
	}

	// Connect with InsecureSkipVerify so we can inspect even invalid certs
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         validationName,
		MinVersion:         tls.VersionTLS10, // inspection dial: reach legacy hosts down to TLS 1.0
	})
	if err != nil {
		return &CheckCertificateResult{
			Error: classifyTLSError(err),
		}
	}
	defer func(conn *tls.Conn) {
		_ = conn.Close()
	}(conn)

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return &CheckCertificateResult{
			Error: "no TLS certificate presented",
		}
	}

	leaf := state.PeerCertificates[0]
	result := extractCertDetails(leaf, state.PeerCertificates)

	// Validate chain
	result.ChainValid, result.ChainError = validateChain(leaf, state.PeerCertificates[1:], validationName)

	// Check hostname match
	result.HostnameMatch = checkHostnameMatch(leaf, validationName)

	applyOCSPStaple(result, state.OCSPResponse, state.PeerCertificates)

	return result
}

// CheckCertificateFromPeerCerts processes pre-fetched TLS peer certificates
// (from an HTTP response) without making a new TLS connection.
func CheckCertificateFromPeerCerts(certs []*x509.Certificate, hostname string, ocspResponse []byte) *CheckCertificateResult {
	if len(certs) == 0 {
		return &CheckCertificateResult{
			Error: "no TLS certificate presented",
		}
	}

	leaf := certs[0]
	result := extractCertDetails(leaf, certs)

	// Validate chain
	result.ChainValid, result.ChainError = validateChain(leaf, certs[1:], hostname)

	// Check hostname match
	result.HostnameMatch = checkHostnameMatch(leaf, hostname)

	applyOCSPStaple(result, ocspResponse, certs)

	return result
}

func applyOCSPStaple(result *CheckCertificateResult, ocspResponse []byte, certs []*x509.Certificate) {
	if len(ocspResponse) == 0 {
		return
	}
	result.OCSPStapled = true
	if len(certs) < 2 {
		result.OCSPStatus = "error"
		result.OCSPError = "issuer certificate not available in chain"
		return
	}
	status, producedAt, nextUpdate, errMsg := parseOCSPResponse(ocspResponse, certs[0], certs[1])
	result.OCSPStatus = status
	result.OCSPProducedAt = producedAt
	result.OCSPNextUpdate = nextUpdate
	result.OCSPError = errMsg
}

func parseOCSPResponse(raw []byte, leaf, issuer *x509.Certificate) (string, *time.Time, *time.Time, string) {
	resp, err := ocsp.ParseResponseForCert(raw, leaf, issuer)
	if err != nil {
		return "error", nil, nil, err.Error()
	}

	producedAt := resp.ProducedAt
	nextUpdate := resp.NextUpdate

	// A stale staple (NextUpdate in the past) downgrades to "unknown" rather
	// than the parsed status — stale OCSP data is not authoritative.
	if !nextUpdate.IsZero() && nextUpdate.Before(time.Now()) {
		return "unknown", &producedAt, &nextUpdate, ""
	}

	switch resp.Status {
	case ocsp.Good:
		return "good", &producedAt, &nextUpdate, ""
	case ocsp.Revoked:
		return "revoked", &producedAt, &nextUpdate, ""
	case ocsp.Unknown:
		return "unknown", &producedAt, &nextUpdate, ""
	default:
		return "error", nil, nil, fmt.Sprintf("unexpected OCSP status: %d", resp.Status)
	}
}

func extractCertDetails(leaf *x509.Certificate, allCerts []*x509.Certificate) *CheckCertificateResult {
	result := &CheckCertificateResult{
		SubjectCN:          leaf.Subject.CommonName,
		IssuerCN:           leaf.Issuer.CommonName,
		SANs:               leaf.DNSNames,
		SerialNumber:       leaf.SerialNumber.Text(16),
		SignatureAlgorithm: leaf.SignatureAlgorithm.String(),
		NotBefore:          leaf.NotBefore,
		NotAfter:           leaf.NotAfter,
	}

	if len(leaf.Issuer.Organization) > 0 {
		result.IssuerOrg = leaf.Issuer.Organization[0]
	}

	// Build chain entries
	for i, cert := range allCerts {
		result.Chain = append(result.Chain, ChainCert{
			SubjectCN: cert.Subject.CommonName,
			IssuerCN:  cert.Issuer.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
		})
		_ = i
	}

	return result
}

func validateChain(leaf *x509.Certificate, intermediates []*x509.Certificate, hostname string) (bool, string) {
	pool := x509.NewCertPool()
	for _, cert := range intermediates {
		pool.AddCert(cert)
	}

	opts := x509.VerifyOptions{
		DNSName:       hostname,
		Intermediates: pool,
		// Roots: nil uses system CA pool
	}

	_, err := leaf.Verify(opts)
	if err == nil {
		return true, ""
	}

	return false, classifyChainError(err)
}

func checkHostnameMatch(leaf *x509.Certificate, hostname string) bool {
	err := leaf.VerifyHostname(hostname)
	return err == nil
}

func classifyChainError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "certificate has expired"):
		return "expired chain certificate"
	case strings.Contains(errStr, "unknown authority"):
		return "untrusted root or missing intermediate"
	case strings.Contains(errStr, "incompatible"):
		return "incompatible certificate"
	default:
		return fmt.Sprintf("chain validation failed: %s", errStr)
	}
}

func classifyTLSError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "deadline exceeded"):
		return "TLS handshake timeout"
	case strings.Contains(errStr, "connection refused"):
		return "connection refused"
	case strings.Contains(errStr, "no such host"):
		return "hostname not found"
	case strings.Contains(errStr, "network is unreachable"):
		return "network unreachable"
	case strings.Contains(errStr, "tls:"):
		return fmt.Sprintf("TLS error: %s", errStr)
	default:
		return fmt.Sprintf("connection failed: %s", errStr)
	}
}
