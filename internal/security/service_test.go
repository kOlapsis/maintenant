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
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestService_NewInsights_TriggersAlert(t *testing.T) {
	svc := NewService(testLogger())

	var alertCalled bool
	var alertInsights []Insight
	var alertIsRecover bool

	svc.SetAlertCallback(func(containerID int64, containerName string, insights []Insight, isRecover bool) {
		alertCalled = true
		alertInsights = insights
		alertIsRecover = isRecover
	})

	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}

	svc.UpdateContainer(1, "test", insights)

	require.True(t, alertCalled)
	require.Len(t, alertInsights, 1)
	assert.False(t, alertIsRecover)
}

func TestService_UnchangedInsights_NoDuplicateAlert(t *testing.T) {
	svc := NewService(testLogger())

	alertCount := 0
	svc.SetAlertCallback(func(containerID int64, containerName string, insights []Insight, isRecover bool) {
		alertCount++
	})

	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}

	svc.UpdateContainer(1, "test", insights)
	svc.UpdateContainer(1, "test", insights) // Same insights again

	assert.Equal(t, 1, alertCount) // Only one alert, not two
}

func TestService_FullResolution_TriggersRecovery(t *testing.T) {
	svc := NewService(testLogger())

	var lastIsRecover bool
	alertCount := 0
	svc.SetAlertCallback(func(containerID int64, containerName string, insights []Insight, isRecover bool) {
		alertCount++
		lastIsRecover = isRecover
	})

	// First: insights detected
	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}
	svc.UpdateContainer(1, "test", insights)

	// Then: all insights resolved
	svc.UpdateContainer(1, "test", nil)

	assert.Equal(t, 2, alertCount) // Alert + Recovery
	assert.True(t, lastIsRecover)
}

func TestService_PartialResolution_NoRecovery(t *testing.T) {
	svc := NewService(testLogger())

	alertCount := 0
	var lastIsRecover bool
	svc.SetAlertCallback(func(containerID int64, containerName string, insights []Insight, isRecover bool) {
		alertCount++
		lastIsRecover = isRecover
	})

	// Two insights
	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
		{Type: PrivilegedContainer, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}
	svc.UpdateContainer(1, "test", insights)

	// One resolved, one remains
	partial := []Insight{
		{Type: PrivilegedContainer, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}
	svc.UpdateContainer(1, "test", partial)

	// 2 alerts: initial + update (changed set). Neither should be recovery.
	assert.Equal(t, 2, alertCount)
	assert.False(t, lastIsRecover) // Still has insights, not a recovery
}

func TestService_NewInsightAdded_TriggersUpdate(t *testing.T) {
	svc := NewService(testLogger())

	alertCount := 0
	svc.SetAlertCallback(func(containerID int64, containerName string, insights []Insight, isRecover bool) {
		alertCount++
	})

	// Start with one insight
	svc.UpdateContainer(1, "test", []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	})

	// Add another
	svc.UpdateContainer(1, "test", []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
		{Type: PrivilegedContainer, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	})

	assert.Equal(t, 2, alertCount) // Initial + Updated
}

func TestService_SSEEvents(t *testing.T) {
	svc := NewService(testLogger())

	var events []string
	svc.SetEventCallback(func(eventType string, data any) {
		events = append(events, eventType)
	})

	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}

	svc.UpdateContainer(1, "test", insights)    // → security.insights_changed
	svc.UpdateContainer(1, "test", nil)          // → security.insights_resolved

	require.Len(t, events, 2)
	assert.Equal(t, "security.insights_changed", events[0])
	assert.Equal(t, "security.insights_resolved", events[1])
}

func TestService_GetContainerInsights(t *testing.T) {
	svc := NewService(testLogger())

	insights := []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
		{Type: PrivilegedContainer, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	}
	svc.UpdateContainer(1, "test", insights)

	ci := svc.GetContainerInsights(1)
	assert.Equal(t, 2, ci.Count)
	assert.NotNil(t, ci.HighestSeverity)
	assert.Equal(t, SeverityCritical, *ci.HighestSeverity)
}

func TestService_GetContainerInsights_NoInsights(t *testing.T) {
	svc := NewService(testLogger())

	ci := svc.GetContainerInsights(99)
	assert.Equal(t, 0, ci.Count)
	assert.Empty(t, ci.Insights)
}

func TestService_GetSummary(t *testing.T) {
	svc := NewService(testLogger())

	svc.UpdateContainer(1, "c1", []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "c1"},
	})
	svc.UpdateContainer(2, "c2", []Insight{
		{Type: HostNetworkMode, Severity: SeverityHigh, ContainerID: 2, ContainerName: "c2"},
	})

	summary := svc.GetSummary(10)
	assert.Equal(t, 10, summary.TotalContainersMonitored)
	assert.Equal(t, 2, summary.TotalContainersAffected)
	assert.Equal(t, 2, summary.TotalInsights)
	assert.Equal(t, 1, summary.BySeverity[SeverityCritical])
	assert.Equal(t, 1, summary.BySeverity[SeverityHigh])
}

func TestService_InsightCount(t *testing.T) {
	svc := NewService(testLogger())

	svc.UpdateContainer(1, "test", []Insight{
		{Type: PortExposedAllInterfaces, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
		{Type: PrivilegedContainer, Severity: SeverityCritical, ContainerID: 1, ContainerName: "test"},
	})

	count, sev := svc.InsightCount(1)
	assert.Equal(t, 2, count)
	assert.Equal(t, SeverityCritical, sev)

	count, sev = svc.InsightCount(99)
	assert.Equal(t, 0, count)
	assert.Equal(t, "", sev)
}

func TestFormatAlertMessage(t *testing.T) {
	insights := []Insight{
		{Type: PrivilegedContainer, Title: "Privileged container", Details: map[string]any{}},
		{Type: PortExposedAllInterfaces, Title: "Port exposed on all interfaces", Details: map[string]any{"port": 6379, "protocol": "tcp"}},
		{Type: DatabasePortExposed, Title: "Database port publicly exposed", Details: map[string]any{"port": 6379, "database_type": "Redis"}},
	}

	msg := FormatAlertMessage(insights)
	assert.Contains(t, msg, "3 security issue(s) detected")
	assert.Contains(t, msg, "Privileged container")
	assert.Contains(t, msg, "Port exposed on all interfaces (6379/tcp)")
	assert.Contains(t, msg, "Database port publicly exposed (Redis 6379)")
}
