// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import (
	"context"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C7, edge case "moniteur de certificat issu de label".
//
// On Linux a TLS dial to 0.0.0.0:port reaches the loopback listener on that port: the unspecified
// address is a refused destination the test can really serve.

func TestCheckCertificateFrom_LabelSourceRefusesAnUnspecifiedAddress(t *testing.T) {
	port, lastSNI := startTLSCapture(t, "guarded.example.internal")
	require.Empty(t, lastSNI(), "no handshake before the probe")

	label := CheckCertificateFrom(SourceLabel, "0.0.0.0", port, "guarded.example.internal", 2*time.Second)
	require.NotEmpty(t, label.Error, "a label-declared monitor must not reach an unspecified address")
	assert.Contains(t, label.Error, "probe guard: blocked connection to 0.0.0.0")
	assert.Empty(t, lastSNI(), "no handshake may have reached the server")
	assert.Empty(t, label.SubjectCN)

	standalone := CheckCertificateFrom(SourceStandalone, "0.0.0.0", port, "guarded.example.internal", 2*time.Second)
	assert.Empty(t, standalone.Error, "an API-created monitor is attempted as today")
	assert.Equal(t, "guarded.example.internal", standalone.SubjectCN)
}

func TestCheckCertificateFrom_LabelSourceStillReachesLoopback(t *testing.T) {
	port, _ := startTLSCapture(t, "lo.example.internal")

	result := CheckCertificateFrom(SourceLabel, "127.0.0.1", port, "lo.example.internal", 2*time.Second)

	require.Empty(t, result.Error)
	assert.Equal(t, "lo.example.internal", result.SubjectCN)
	assert.True(t, result.HostnameMatch)
}

func TestCheckCertificateFrom_LabelSourceRefusesLinkLocal(t *testing.T) {
	result := CheckCertificateFrom(SourceLabel, "169.254.169.254", 443, "", time.Second)

	assert.Contains(t, result.Error, "probe guard: blocked connection to 169.254.169.254")
}

// FR-013: the opt-out lifts the rule for certificates too.
func TestCheckCertificateFrom_OptOutLiftsTheGuard(t *testing.T) {
	t.Setenv("MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL", "true")
	port, _ := startTLSCapture(t, "optout.example.internal")

	result := CheckCertificateFrom(SourceLabel, "0.0.0.0", port, "optout.example.internal", 2*time.Second)

	require.Empty(t, result.Error)
	assert.Equal(t, "optout.example.internal", result.SubjectCN)
}

// CheckCertificate keeps today's behaviour: no guard.
func TestCheckCertificate_IsNotGuarded(t *testing.T) {
	port, _ := startTLSCapture(t, "plain.example.internal")

	result := CheckCertificate("0.0.0.0", port, "plain.example.internal", 2*time.Second)

	require.Empty(t, result.Error)
	assert.Equal(t, "plain.example.internal", result.SubjectCN)
}

// Edge case "un moniteur passé de label à API": the service checks with the monitor's current source.
func TestCheckNow_LabelMonitorIsGuardedAndStandaloneIsNot(t *testing.T) {
	_, port := tlsTestTarget(t)

	monitor := func(source CertSource) *CertMonitor {
		return &CertMonitor{
			Hostname: "0.0.0.0", Port: port, Source: source, Status: StatusUnknown,
			CheckIntervalSeconds: 86400, WarningThresholds: DefaultWarningThresholds(), AgentID: uid.LocalAgent,
		}
	}

	labelSvc, labelID := checkNowService(t, monitor(SourceLabel))
	labelled, err := labelSvc.CheckNow(context.Background(), labelID)
	require.NoError(t, err)
	assert.Equal(t, StatusError, labelled.Status)
	assert.Contains(t, labelled.LastError, "probe guard")

	apiSvc, apiID := checkNowService(t, monitor(SourceStandalone))
	api, err := apiSvc.CheckNow(context.Background(), apiID)
	require.NoError(t, err)
	assert.NotEqual(t, StatusError, api.Status, api.LastError)
	assert.Empty(t, api.LastError)
}
