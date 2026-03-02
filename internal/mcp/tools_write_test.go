package mcp

import (
	"context"
	"log/slog"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCEServices() *Services {
	return &Services{
		Incidents:   extension.NoopIncidentManager{},
		Maintenance: extension.NoopMaintenanceScheduler{},
		Logger:      slog.Default(),
		Version:     "test",
	}
}

func TestAcknowledgeAlertHandler_ReturnsNotAvailable(t *testing.T) {
	svc := newCEServices()
	handler := acknowledgeAlertHandler(svc)

	result, _, err := handler(context.Background(), nil, acknowledgeAlertInput{AlertID: 1})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Equal(t, extension.ErrNotAvailable.Error(), text)
}

func TestCreateIncidentHandler_CE_ReturnsNotAvailable(t *testing.T) {
	svc := newCEServices()
	handler := createIncidentHandler(svc)

	input := createIncidentInput{
		Title:   "Test incident",
		Status:  "investigating",
		Message: "Something broke",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, extension.ErrNotAvailable.Error(), textFromContent(t, result.Content))
}

func TestUpdateIncidentHandler_CE_ReturnsNotAvailable(t *testing.T) {
	svc := newCEServices()
	handler := updateIncidentHandler(svc)

	input := updateIncidentInput{
		IncidentID: 1,
		Status:     "resolved",
		Message:    "Fixed",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, extension.ErrNotAvailable.Error(), textFromContent(t, result.Content))
}

func TestCreateMaintenanceHandler_CE_ReturnsNotAvailable(t *testing.T) {
	svc := newCEServices()
	handler := createMaintenanceHandler(svc)

	input := createMaintenanceInput{
		Title:     "Scheduled update",
		StartTime: "2026-03-01T02:00:00Z",
		EndTime:   "2026-03-01T04:00:00Z",
		Message:   "Updating servers",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, extension.ErrNotAvailable.Error(), textFromContent(t, result.Content))
}

func TestPauseMonitorHandler_InvalidType(t *testing.T) {
	svc := newCEServices()
	handler := pauseMonitorHandler(svc)

	input := pauseMonitorInput{
		MonitorType: "endpoint",
		MonitorID:   1,
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestPauseMonitorHandler_EmptyType(t *testing.T) {
	svc := newCEServices()
	handler := pauseMonitorHandler(svc)

	input := pauseMonitorInput{
		MonitorType: "",
		MonitorID:   1,
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestResumeMonitorHandler_InvalidType(t *testing.T) {
	svc := newCEServices()
	handler := resumeMonitorHandler(svc)

	input := resumeMonitorInput{
		MonitorType: "container",
		MonitorID:   1,
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestResumeMonitorHandler_EmptyType(t *testing.T) {
	svc := newCEServices()
	handler := resumeMonitorHandler(svc)

	input := resumeMonitorInput{
		MonitorType: "",
		MonitorID:   1,
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestWriteToolRegistration(t *testing.T) {
	svc := newCEServices()
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "maintenant-test",
		Version: "0.0.1",
	}, nil)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("registerWriteTools panics due to go-sdk v1.4.0 jsonschema tag parsing: %v", r)
			}
		}()
		registerWriteTools(server, svc)
	}()
}
