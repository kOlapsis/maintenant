// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

func TestAnalyzeDocker_PortExposedOnAllInterfaces(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "0.0.0.0", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)

	require.Len(t, insights, 1)
	assert.Equal(t, PortExposedAllInterfaces, insights[0].Type)
	assert.Equal(t, SeverityCritical, insights[0].Severity)
	assert.Equal(t, 8080, insights[0].Details["port"])
	assert.Equal(t, "tcp", insights[0].Details["protocol"])
}

func TestAnalyzeDocker_PortOnLocalhost_NoInsight(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "127.0.0.1", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)
	assert.Empty(t, insights)
}

func TestAnalyzeDocker_PortOnSpecificIP_NoInsight(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "192.168.1.100", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)
	assert.Empty(t, insights)
}

func TestAnalyzeDocker_EmptyHostIP_TreatedAsAllInterfaces(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)
	require.Len(t, insights, 1)
	assert.Equal(t, PortExposedAllInterfaces, insights[0].Type)
}

func TestAnalyzeDocker_IPv6AllInterfaces(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "::", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)
	require.Len(t, insights, 1)
	assert.Equal(t, PortExposedAllInterfaces, insights[0].Type)
}

func TestAnalyzeDocker_DatabasePortExposed(t *testing.T) {
	tests := []struct {
		port   int
		dbType string
	}{
		{3306, "MySQL/MariaDB"},
		{5432, "PostgreSQL"},
		{6379, "Redis"},
		{27017, "MongoDB"},
	}

	for _, tt := range tests {
		t.Run(tt.dbType, func(t *testing.T) {
			cfg := DockerSecurityConfig{
				Bindings: []PortBinding{
					{HostIP: "0.0.0.0", HostPort: "0", Port: tt.port, Protocol: "tcp"},
				},
			}

			insights := AnalyzeDocker(1, "test-db", cfg, testNow)

			// Should have both PortExposedAllInterfaces AND DatabasePortExposed
			require.Len(t, insights, 2)
			assert.Equal(t, PortExposedAllInterfaces, insights[0].Type)
			assert.Equal(t, DatabasePortExposed, insights[1].Type)
			assert.Equal(t, tt.dbType, insights[1].Details["database_type"])
		})
	}
}

func TestAnalyzeDocker_NonStandardDBPort_NoDatabaseInsight(t *testing.T) {
	cfg := DockerSecurityConfig{
		Bindings: []PortBinding{
			{HostIP: "0.0.0.0", HostPort: "16379", Port: 16379, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-redis", cfg, testNow)

	// Only PortExposedAllInterfaces, NOT DatabasePortExposed
	require.Len(t, insights, 1)
	assert.Equal(t, PortExposedAllInterfaces, insights[0].Type)
}

func TestAnalyzeDocker_HostNetworkMode(t *testing.T) {
	cfg := DockerSecurityConfig{
		NetworkMode: "host",
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)

	require.Len(t, insights, 1)
	assert.Equal(t, HostNetworkMode, insights[0].Type)
	assert.Equal(t, SeverityHigh, insights[0].Severity)
}

func TestAnalyzeDocker_HostNetworkMode_SkipsPortBindings(t *testing.T) {
	cfg := DockerSecurityConfig{
		NetworkMode: "host",
		Bindings: []PortBinding{
			{HostIP: "0.0.0.0", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)

	// Only host network mode, NOT port exposed
	require.Len(t, insights, 1)
	assert.Equal(t, HostNetworkMode, insights[0].Type)
}

func TestAnalyzeDocker_Privileged(t *testing.T) {
	cfg := DockerSecurityConfig{
		Privileged: true,
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)

	require.Len(t, insights, 1)
	assert.Equal(t, PrivilegedContainer, insights[0].Type)
	assert.Equal(t, SeverityCritical, insights[0].Severity)
}

func TestAnalyzeDocker_MultipleIssues(t *testing.T) {
	cfg := DockerSecurityConfig{
		Privileged: true,
		Bindings: []PortBinding{
			{HostIP: "0.0.0.0", HostPort: "6379", Port: 6379, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)

	// PortExposedAllInterfaces + DatabasePortExposed + PrivilegedContainer
	require.Len(t, insights, 3)

	types := make(map[InsightType]bool)
	for _, i := range insights {
		types[i.Type] = true
	}
	assert.True(t, types[PortExposedAllInterfaces])
	assert.True(t, types[DatabasePortExposed])
	assert.True(t, types[PrivilegedContainer])
}

func TestAnalyzeDocker_NoIssues(t *testing.T) {
	cfg := DockerSecurityConfig{
		NetworkMode: "bridge",
		Bindings: []PortBinding{
			{HostIP: "127.0.0.1", HostPort: "8080", Port: 8080, Protocol: "tcp"},
		},
	}

	insights := AnalyzeDocker(1, "test-container", cfg, testNow)
	assert.Empty(t, insights)
}
