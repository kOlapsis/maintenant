// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/status"
)

func TestCertificateStatus(t *testing.T) {
	checked := time.Now()
	tests := []struct {
		name string
		cert certificate.CertMonitor
		want string
	}{
		{"valid", certificate.CertMonitor{Status: certificate.StatusValid, LastCheckAt: &checked}, status.StatusOperational},
		{"expiring", certificate.CertMonitor{Status: certificate.StatusExpiring, LastCheckAt: &checked}, status.StatusDegraded},
		{"expired", certificate.CertMonitor{Status: certificate.StatusExpired, LastCheckAt: &checked}, status.StatusMajorOutage},
		{"error", certificate.CertMonitor{Status: certificate.StatusError, LastCheckAt: &checked, LastError: "dial tcp: timeout"}, status.StatusMajorOutage},
		{"never checked", certificate.CertMonitor{Status: certificate.StatusUnknown}, status.StatusOperational},
		{"unknown after a check", certificate.CertMonitor{Status: certificate.StatusUnknown, LastCheckAt: &checked}, status.StatusMajorOutage},
		{"unknown with an error", certificate.CertMonitor{Status: certificate.StatusUnknown, LastError: "dial tcp: timeout"}, status.StatusMajorOutage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CertificateStatus(&tt.cert))
		})
	}
}

func TestEndpointStatus(t *testing.T) {
	checked := time.Now()
	tests := []struct {
		name string
		ep   endpoint.Endpoint
		want string
	}{
		{"up", endpoint.Endpoint{Status: endpoint.StatusUp, LastCheckAt: &checked}, status.StatusOperational},
		{"down", endpoint.Endpoint{Status: endpoint.StatusDown, LastCheckAt: &checked}, status.StatusMajorOutage},
		{"degraded", endpoint.Endpoint{Status: endpoint.StatusDegraded, LastCheckAt: &checked}, status.StatusDegraded},
		{"never checked", endpoint.Endpoint{Status: endpoint.StatusUnknown}, status.StatusOperational},
		{"unknown after a check", endpoint.Endpoint{Status: endpoint.StatusUnknown, LastCheckAt: &checked}, status.StatusMajorOutage},
		{"unknown with an error", endpoint.Endpoint{Status: endpoint.StatusUnknown, LastError: "container stopped"}, status.StatusMajorOutage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EndpointStatus(&tt.ep))
		})
	}
}

func TestHeartbeatStatus(t *testing.T) {
	tests := []struct {
		status heartbeat.HeartbeatStatus
		want   string
	}{
		{heartbeat.StatusUp, status.StatusOperational},
		{heartbeat.StatusDown, status.StatusMajorOutage},
		{heartbeat.StatusStarted, status.StatusOperational},
		{heartbeat.StatusNew, status.StatusOperational},
		{heartbeat.StatusPaused, status.StatusOperational},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.want, HeartbeatStatus(&heartbeat.Heartbeat{Status: tt.status}))
		})
	}
}
