package certificate

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// IsAutoRenewable — detect ACME-based auto-renewable issuers
// ---------------------------------------------------------------------------

func TestIsAutoRenewable(t *testing.T) {
	tests := []struct {
		issuerOrg string
		want      bool
	}{
		{"Let's Encrypt", true},
		{"let's encrypt", true},
		{"LET'S ENCRYPT", true},
		{"ZeroSSL", true},
		{"Buypass", true},
		{"Google Trust Services LLC", true},
		{"DigiCert Inc", false},
		{"Sectigo Limited", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.issuerOrg, func(t *testing.T) {
			assert.Equal(t, tt.want, IsAutoRenewable(tt.issuerOrg))
		})
	}
}

// ---------------------------------------------------------------------------
// ExpiringSeverity — severity based on days remaining + issuer
// ---------------------------------------------------------------------------

func TestExpiringSeverity_AutoRenewable(t *testing.T) {
	// Let's Encrypt: only escalates when auto-renewal clearly failed
	assert.Equal(t, "info", ExpiringSeverity(30, "Let's Encrypt"))
	assert.Equal(t, "info", ExpiringSeverity(14, "Let's Encrypt"))
	assert.Equal(t, "info", ExpiringSeverity(8, "Let's Encrypt"))
	assert.Equal(t, "warning", ExpiringSeverity(7, "Let's Encrypt"))
	assert.Equal(t, "warning", ExpiringSeverity(4, "Let's Encrypt"))
	assert.Equal(t, "critical", ExpiringSeverity(3, "Let's Encrypt"))
	assert.Equal(t, "critical", ExpiringSeverity(1, "Let's Encrypt"))
}

func TestExpiringSeverity_NormalCert(t *testing.T) {
	// Standard CA: progressive escalation
	assert.Equal(t, "info", ExpiringSeverity(30, "DigiCert Inc"))
	assert.Equal(t, "warning", ExpiringSeverity(14, "DigiCert Inc"))
	assert.Equal(t, "warning", ExpiringSeverity(8, "DigiCert Inc"))
	assert.Equal(t, "critical", ExpiringSeverity(7, "DigiCert Inc"))
	assert.Equal(t, "critical", ExpiringSeverity(3, "DigiCert Inc"))
	assert.Equal(t, "critical", ExpiringSeverity(1, "DigiCert Inc"))
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

	assert.Equal(t, 443, byHost["example.com"].Port)      // default port
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

// ---------------------------------------------------------------------------
// Mock store for quota testing
// ---------------------------------------------------------------------------

type mockCertStore struct {
	monitors               map[string]*CertMonitor
	standaloneCount        int
	getMonitorByHostPortFn func(ctx context.Context, hostname string, port int, serverName string) (*CertMonitor, error)
}

func newMockCertStore() *mockCertStore {
	return &mockCertStore{
		monitors: make(map[string]*CertMonitor),
	}
}

func (m *mockCertStore) CreateMonitor(_ context.Context, monitor *CertMonitor) (string, error) {
	monitor.ID = uid.New()
	m.monitors[monitor.ID] = monitor
	if monitor.Source == SourceStandalone {
		m.standaloneCount++
	}
	return monitor.ID, nil
}

func (m *mockCertStore) GetMonitorByHostPort(ctx context.Context, hostname string, port int, serverName string) (*CertMonitor, error) {
	if m.getMonitorByHostPortFn != nil {
		return m.getMonitorByHostPortFn(ctx, hostname, port, serverName)
	}
	return nil, nil
}

func (m *mockCertStore) GetMonitorByHostPortAgent(ctx context.Context, _ *string, hostname string, port int, serverName string) (*CertMonitor, error) {
	if m.getMonitorByHostPortFn != nil {
		return m.getMonitorByHostPortFn(ctx, hostname, port, serverName)
	}
	return nil, nil
}

func (m *mockCertStore) CountStandaloneMonitors(_ context.Context) (int, error) {
	return m.standaloneCount, nil
}

// Stub implementations for required interface methods
func (m *mockCertStore) GetMonitorByID(_ context.Context, id string) (*CertMonitor, error) {
	return m.monitors[id], nil
}
func (m *mockCertStore) GetMonitorByEndpointID(_ context.Context, _ string) (*CertMonitor, error) {
	return nil, nil
}
func (m *mockCertStore) ListMonitors(_ context.Context, _ ListCertificatesOpts) ([]*CertMonitor, error) {
	return nil, nil
}
func (m *mockCertStore) UpdateMonitor(_ context.Context, _ *CertMonitor) error {
	return nil
}
func (m *mockCertStore) DeleteMonitor(_ context.Context, _ string) error {
	return nil
}
func (m *mockCertStore) InsertCheckResult(_ context.Context, _ *CertCheckResult) (string, error) {
	return "", nil
}
func (m *mockCertStore) GetLatestCheckResult(_ context.Context, _ string) (*CertCheckResult, error) {
	return nil, nil
}
func (m *mockCertStore) ListCheckResults(_ context.Context, _ string, _ ListChecksOpts) ([]*CertCheckResult, int, error) {
	return nil, 0, nil
}
func (m *mockCertStore) InsertChainEntries(_ context.Context, _ []*CertChainEntry) error {
	return nil
}
func (m *mockCertStore) GetChainEntries(_ context.Context, _ string) ([]*CertChainEntry, error) {
	return nil, nil
}
func (m *mockCertStore) ListMonitorsByExternalID(_ context.Context, _ string) ([]*CertMonitor, error) {
	return nil, nil
}
func (m *mockCertStore) ListDueScheduledMonitors(_ context.Context, _ time.Time) ([]*CertMonitor, error) {
	return nil, nil
}
func (m *mockCertStore) DeleteCheckResultsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---------------------------------------------------------------------------
// Quota enforcement test
// ---------------------------------------------------------------------------

func TestService_CreateStandalone_QuotaEnforced(t *testing.T) {
	store := newMockCertStore()
	svc := NewService(Deps{
		Store:          store,
		Logger:         noopLogger(),
		LicenseChecker: &DefaultLicenseChecker{MaxCertificates: 2},
	})
	ctx := context.Background()

	// Create first monitor - should succeed
	monitor1, _, err := svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname:             "example.com",
		Port:                 443,
		CheckIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	assert.NotNil(t, monitor1)
	assert.Equal(t, "example.com", monitor1.Hostname)

	// Create second monitor - should succeed
	monitor2, _, err := svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname:             "example.org",
		Port:                 443,
		CheckIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	assert.NotNil(t, monitor2)

	// Create third monitor - should fail due to quota
	monitor3, _, err := svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname:             "example.net",
		Port:                 443,
		CheckIntervalSeconds: 3600,
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLimitReached), "expected ErrLimitReached")
	assert.Nil(t, monitor3)

	// Verify count
	count, err := store.CountStandaloneMonitors(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// ---------------------------------------------------------------------------
// SNI (server_name) tests
// ---------------------------------------------------------------------------

func TestService_CreateStandalone_InvalidServerName(t *testing.T) {
	svc := NewService(Deps{Store: newMockCertStore(), Logger: noopLogger()})

	for _, sni := range []string{"bad:443", "https://bad", "bad name"} {
		_, _, err := svc.CreateStandalone(context.Background(), CreateCertificateInput{
			Hostname:             "proxy.invalid",
			Port:                 443,
			ServerName:           sni,
			CheckIntervalSeconds: 3600,
		})
		assert.True(t, errors.Is(err, ErrInvalidInput), "server_name %q should be rejected", sni)
	}
}

func TestService_CreateStandalone_SNICoexistsAndDedups(t *testing.T) {
	store := newMockCertStore()
	// Mirror the real store lookup: an existing monitor only matches on the
	// full (hostname, port, server_name) identity.
	store.getMonitorByHostPortFn = func(_ context.Context, hostname string, port int, serverName string) (*CertMonitor, error) {
		for _, m := range store.monitors {
			if m.Hostname == hostname && m.Port == port && m.ServerName == serverName {
				return m, nil
			}
		}
		return nil, nil
	}
	svc := NewService(Deps{Store: store, Logger: noopLogger()})
	ctx := context.Background()

	// SNI-less monitor, then an SNI monitor on the same host:port — both live.
	_, _, err := svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname: "proxy2.invalid", Port: 443, CheckIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	withSNI, _, err := svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname: "proxy2.invalid", Port: 443, ServerName: "service.example.invalid", CheckIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	assert.Equal(t, "service.example.invalid", withSNI.ServerName)

	// Same (host, port, server_name) again is a duplicate.
	_, _, err = svc.CreateStandalone(ctx, CreateCertificateInput{
		Hostname: "proxy2.invalid", Port: 443, ServerName: "service.example.invalid", CheckIntervalSeconds: 3600,
	})
	assert.True(t, errors.Is(err, ErrDuplicateMonitor), "expected ErrDuplicateMonitor, got %v", err)
}

// TestDefaultLicenseChecker_Unlimited: -1 means no cap, not a cap of -1.
func TestDefaultLicenseChecker_Unlimited(t *testing.T) {
	c := &DefaultLicenseChecker{MaxCertificates: -1}
	for _, count := range []int{0, 1, 5, 500} {
		if !c.CanCreateCertificate(count) {
			t.Errorf("CanCreateCertificate(%d) = false with an unlimited cap", count)
		}
	}
}

func TestDefaultLicenseChecker_Capped(t *testing.T) {
	c := &DefaultLicenseChecker{MaxCertificates: 5}
	if !c.CanCreateCertificate(4) {
		t.Error("the fifth monitor must be allowed")
	}
	if c.CanCreateCertificate(5) {
		t.Error("the sixth monitor must be refused")
	}
}

// ---------------------------------------------------------------------------
// CheckNow — issue #44: confirm a fix without waiting for the next daily check
// ---------------------------------------------------------------------------

// checkNowService returns a service over a mock store holding monitor, and the
// id the monitor was stored under.
func checkNowService(t *testing.T, monitor *CertMonitor) (*Service, string) {
	t.Helper()
	svc, id, _ := checkNowServiceWithEvents(t, monitor)
	return svc, id
}

// checkNowServiceWithEvents also returns a recorder of the emitted event types.
func checkNowServiceWithEvents(t *testing.T, monitor *CertMonitor) (*Service, string, func() []string) {
	t.Helper()
	store := newMockCertStore()
	monitor.ID = uid.New()
	store.monitors[monitor.ID] = monitor

	var mu sync.Mutex
	var events []string
	svc := NewService(Deps{
		Store:  store,
		Logger: noopLogger(),
		EventCallback: func(eventType string, _ interface{}) {
			mu.Lock()
			events = append(events, eventType)
			mu.Unlock()
		},
	})
	return svc, monitor.ID, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), events...)
	}
}

// tlsTestTarget starts a TLS server and returns its host and port.
func tlsTestTarget(t *testing.T) (string, int) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return host, port
}

// A renewed certificate must be confirmable on the spot: the scan runs, the
// status is recomputed and the next scheduled check is pushed a full interval out.
func TestCheckNow_ScansAndRefreshesTheMonitor(t *testing.T) {
	host, port := tlsTestTarget(t)

	svc, id := checkNowService(t, &CertMonitor{
		Hostname:             host,
		Port:                 port,
		Source:               SourceStandalone,
		Status:               StatusError,
		LastError:            "dial failed",
		CheckIntervalSeconds: 86400,
		WarningThresholds:    DefaultWarningThresholds(),
		AgentID:              uid.LocalAgent,
	})

	monitor, err := svc.CheckNow(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, monitor.LastCheckAt)
	assert.Empty(t, monitor.LastError, "a successful scan must clear the previous error")
	assert.NotEqual(t, StatusError, monitor.Status, "the status must be recomputed from the scan")
	require.NotNil(t, monitor.NextCheckAt)
	assert.True(t, monitor.NextCheckAt.After(time.Now().Add(23*time.Hour)),
		"the next scheduled check must be pushed a full interval out")
}

// A monitor an agent scans is unreachable from the server: better a clear refusal
// than a check that dials nothing and reports a false failure.
func TestCheckNow_RefusesAnAgentScannedMonitor(t *testing.T) {
	svc, id := checkNowService(t, &CertMonitor{
		Hostname: "internal.example.invalid", Port: 443,
		Source: SourceLabel, AgentID: uid.New(),
	})

	_, err := svc.CheckNow(context.Background(), id)
	assert.ErrorIs(t, err, ErrAgentScanned)
}

func TestCheckNow_UnknownMonitor(t *testing.T) {
	svc := NewService(Deps{Store: newMockCertStore(), Logger: noopLogger()})

	_, err := svc.CheckNow(context.Background(), uid.New())
	assert.ErrorIs(t, err, ErrMonitorNotFound)
}

// Two clicks must not mean two concurrent dials at the same target.
func TestCheckNow_RefusesAConcurrentCheck(t *testing.T) {
	svc, id := checkNowService(t, &CertMonitor{
		Hostname: "example.invalid", Port: 443,
		Source: SourceStandalone, AgentID: uid.LocalAgent,
	})
	svc.manualChecks.Store(id, struct{}{})

	_, err := svc.CheckNow(context.Background(), id)
	assert.ErrorIs(t, err, ErrCheckInProgress)
}

// A status change must reach the UI: previousStatus is captured before the scan
// overwrites it, otherwise the comparison is always false and the SSE event that
// refreshes the certificate list never leaves the server.
func TestProcessCheckResult_EmitsStatusChanged(t *testing.T) {
	host, port := tlsTestTarget(t)

	svc, id, events := checkNowServiceWithEvents(t, &CertMonitor{
		Hostname:             host,
		Port:                 port,
		Source:               SourceStandalone,
		Status:               StatusError,
		CheckIntervalSeconds: 86400,
		WarningThresholds:    DefaultWarningThresholds(),
		AgentID:              uid.LocalAgent,
	})

	monitor, err := svc.CheckNow(context.Background(), id)
	require.NoError(t, err)
	require.NotEqual(t, StatusError, monitor.Status, "the scan must move the monitor off error")
	assert.Contains(t, events(), event.CertificateStatusChanged,
		"a status change must be broadcast")
}

// The same scan twice in a row is not a change, and must not be announced as one.
func TestProcessCheckResult_NoEventWhenStatusHolds(t *testing.T) {
	host, port := tlsTestTarget(t)

	svc, id, events := checkNowServiceWithEvents(t, &CertMonitor{
		Hostname:             host,
		Port:                 port,
		Source:               SourceStandalone,
		Status:               StatusError,
		CheckIntervalSeconds: 86400,
		WarningThresholds:    DefaultWarningThresholds(),
		AgentID:              uid.LocalAgent,
	})

	_, err := svc.CheckNow(context.Background(), id)
	require.NoError(t, err)
	first := len(events())

	_, err = svc.CheckNow(context.Background(), id)
	require.NoError(t, err)

	var changed int
	for _, e := range events()[first:] {
		if e == event.CertificateStatusChanged {
			changed++
		}
	}
	assert.Zero(t, changed, "an unchanged status must not be announced again")
}
