package certificate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// computeStatus — determines monitor status from cert data + thresholds
// ---------------------------------------------------------------------------

func newTestService() *Service {
	return &Service{}
}

func TestComputeStatus_ValidCert(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(90 * 24 * time.Hour), // 90 days remaining
	}

	status := svc.computeStatus(raw, DefaultWarningThresholds())

	assert.Equal(t, StatusValid, status)
}

func TestComputeStatus_ExpiringAt30Days(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(29 * 24 * time.Hour), // 29 days remaining, crosses 30-day threshold
	}

	status := svc.computeStatus(raw, DefaultWarningThresholds())

	assert.Equal(t, StatusExpiring, status)
}

func TestComputeStatus_ExpiringAt7Days(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(5 * 24 * time.Hour), // 5 days remaining
	}

	status := svc.computeStatus(raw, []int{30, 14, 7, 3, 1})

	assert.Equal(t, StatusExpiring, status)
}

func TestComputeStatus_Expired(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(-24 * time.Hour), // expired yesterday
	}

	status := svc.computeStatus(raw, DefaultWarningThresholds())

	assert.Equal(t, StatusExpired, status)
}

func TestComputeStatus_JustAboveThreshold_StaysValid(t *testing.T) {
	svc := newTestService()
	// 31 days remaining — daysRemaining = int(31*24*hours / 24) = 31, and 31 <= 30 is false → valid
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(32 * 24 * time.Hour), // 32 days to be safely above
	}

	status := svc.computeStatus(raw, []int{30})

	assert.Equal(t, StatusValid, status)
}

func TestComputeStatus_ExactlyAtThreshold_IsExpiring(t *testing.T) {
	svc := newTestService()
	// Exactly 30 days remaining — should trigger at the 30-day threshold
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(30 * 24 * time.Hour),
	}

	status := svc.computeStatus(raw, []int{30})

	assert.Equal(t, StatusExpiring, status)
}

func TestComputeStatus_CustomThresholds(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(45 * 24 * time.Hour),
	}

	// With a 60-day threshold, 45 days remaining is expiring
	status := svc.computeStatus(raw, []int{60})
	assert.Equal(t, StatusExpiring, status)

	// With a 30-day threshold, 45 days remaining is valid
	status = svc.computeStatus(raw, []int{30})
	assert.Equal(t, StatusValid, status)
}

func TestComputeStatus_EmptyThresholds_AlwaysValid(t *testing.T) {
	svc := newTestService()
	raw := &CheckCertificateResult{
		NotAfter: time.Now().Add(5 * 24 * time.Hour), // 5 days
	}

	// No thresholds configured — never transitions to expiring
	status := svc.computeStatus(raw, []int{})

	assert.Equal(t, StatusValid, status)
}

// ---------------------------------------------------------------------------
// extractHostPort — URL parsing for auto-detection
// ---------------------------------------------------------------------------

func TestExtractHostPort_StandardHTTPS(t *testing.T) {
	hostname, port, err := extractHostPort("https://example.com/path")

	assert.NoError(t, err)
	assert.Equal(t, "example.com", hostname)
	assert.Equal(t, 443, port)
}

func TestExtractHostPort_CustomPort(t *testing.T) {
	hostname, port, err := extractHostPort("https://example.com:8443/path")

	assert.NoError(t, err)
	assert.Equal(t, "example.com", hostname)
	assert.Equal(t, 8443, port)
}

func TestExtractHostPort_HTTPRejected(t *testing.T) {
	_, _, err := extractHostPort("http://example.com/path")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not an HTTPS URL")
}

func TestExtractHostPort_EmptyHostnameRejected(t *testing.T) {
	_, _, err := extractHostPort("https:///path")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no hostname")
}

func TestExtractHostPort_InvalidURLRejected(t *testing.T) {
	_, _, err := extractHostPort("://broken")

	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// IsHTTPS
// ---------------------------------------------------------------------------

func TestIsHTTPS(t *testing.T) {
	assert.True(t, IsHTTPS("https://example.com"))
	assert.True(t, IsHTTPS("https://example.com:8443/path?q=1"))
	assert.False(t, IsHTTPS("http://example.com"))
	assert.False(t, IsHTTPS("tcp://example.com:443"))
	assert.False(t, IsHTTPS(""))
}

// ---------------------------------------------------------------------------
// ParseCertificateLabels — label discovery
// ---------------------------------------------------------------------------

func TestParseCertificateLabels_ValidLabels(t *testing.T) {
	labels := map[string]string{
		"maintenant.tls.certificates": "example.com,api.example.com:8443",
	}

	parsed := ParseCertificateLabels(labels)

	assert.Len(t, parsed, 2)

	byHost := make(map[string]ParsedCertLabel)
	for _, p := range parsed {
		byHost[p.Hostname] = p
	}

	assert.Equal(t, 443, byHost["example.com"].Port)   // default port
	assert.Equal(t, 8443, byHost["api.example.com"].Port) // explicit port
}

func TestParseCertificateLabels_NoTLSLabels(t *testing.T) {
	labels := map[string]string{
		"maintenant.endpoint.0.target": "https://example.com",
	}

	parsed := ParseCertificateLabels(labels)

	assert.Empty(t, parsed)
}

func TestParseCertificateLabels_DeduplicatesEntries(t *testing.T) {
	labels := map[string]string{
		"maintenant.tls.certificates": "example.com,example.com,example.com:443",
	}

	parsed := ParseCertificateLabels(labels)

	assert.Len(t, parsed, 1)
}

func TestParseCertificateLabels_StripsSchemeAndPath(t *testing.T) {
	labels := map[string]string{
		"maintenant.tls.certificates": "https://example.com/path",
	}

	parsed := ParseCertificateLabels(labels)

	assert.Len(t, parsed, 1)
	assert.Equal(t, "example.com", parsed[0].Hostname)
	assert.Equal(t, 443, parsed[0].Port)
}
