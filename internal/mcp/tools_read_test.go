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

package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/uid"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textFromContent extracts the text from the first content item, asserting it is a TextContent.
func textFromContent(t *testing.T, content []gomcp.Content) string {
	t.Helper()
	require.NotEmpty(t, content)
	tc, ok := content[0].(*gomcp.TextContent)
	require.True(t, ok, "expected *gomcp.TextContent, got %T", content[0])
	return tc.Text
}

func TestAgentFilter(t *testing.T) {
	assert.Nil(t, agentFilter(""))

	local := agentFilter("local")
	require.NotNil(t, local)
	assert.Equal(t, uid.LocalAgent, *local)

	specific := agentFilter("agent-123")
	require.NotNil(t, specific)
	assert.Equal(t, "agent-123", *specific)
}

func TestJsonResult_MarshalMap(t *testing.T) {
	input := map[string]any{
		"status":  "ok",
		"version": "1.0.0",
	}
	result, _, err := jsonResult(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	var decoded map[string]any
	text := textFromContent(t, result.Content)
	err = json.Unmarshal([]byte(text), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "ok", decoded["status"])
	assert.Equal(t, "1.0.0", decoded["version"])
}

func TestJsonResult_MarshalSlice(t *testing.T) {
	input := []string{"a", "b", "c"}
	result, _, err := jsonResult(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	var decoded []string
	text := textFromContent(t, result.Content)
	err = json.Unmarshal([]byte(text), &decoded)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, decoded)
}

func TestJsonResult_NilValue(t *testing.T) {
	result, _, err := jsonResult(nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "null", textFromContent(t, result.Content))
}

func TestJsonResult_EmptySlice(t *testing.T) {
	result, _, err := jsonResult([]any{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "[]", textFromContent(t, result.Content))
}

func TestTextResult(t *testing.T) {
	result, _, err := textResult("hello world")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "hello world", textFromContent(t, result.Content))
}

func TestTextResult_EmptyString(t *testing.T) {
	result, _, err := textResult("")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "", textFromContent(t, result.Content))
}

func TestTextResult_MultiLineContent(t *testing.T) {
	text := "line1\nline2\nline3"
	result, _, err := textResult(text)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, text, textFromContent(t, result.Content))
}

func TestErrResult(t *testing.T) {
	result, _, err := errResult("something went wrong")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "something went wrong", textFromContent(t, result.Content))
}

func TestErrResult_ContainerNotFound(t *testing.T) {
	result, _, err := errResult("not found: container does not exist")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "not found: container does not exist", textFromContent(t, result.Content))
}

func TestErrResult_InvalidInput(t *testing.T) {
	result, _, err := errResult("invalid input: container_id must be a valid integer")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: container_id must be a valid integer", textFromContent(t, result.Content))
}

// mcpListAlertsSpy wraps mcpAlertStore to record which listing method the
// handler called, so the active_only tri-state (nil/true/false) can be pinned.
type mcpListAlertsSpy struct {
	*mcpAlertStore
	activeOnlyCalled bool
	listAlertsCalled bool
	listAlertsOpts   alert.ListAlertsOpts
}

func newMCPListAlertsSpy() *mcpListAlertsSpy {
	return &mcpListAlertsSpy{mcpAlertStore: newMCPAlertStore()}
}

func (m *mcpListAlertsSpy) ListActiveAlerts(ctx context.Context) ([]*alert.Alert, error) {
	m.activeOnlyCalled = true
	return m.mcpAlertStore.ListActiveAlerts(ctx)
}

func (m *mcpListAlertsSpy) ListAlerts(ctx context.Context, opts alert.ListAlertsOpts) ([]*alert.Alert, error) {
	m.listAlertsCalled = true
	m.listAlertsOpts = opts
	return m.mcpAlertStore.ListAlerts(ctx, opts)
}

func TestListAlertsHandler_DefaultActiveOnly(t *testing.T) {
	spy := newMCPListAlertsSpy()
	svc := &Services{Alerts: spy, Logger: slog.Default(), Version: "test"}

	_, _, err := listAlertsHandler(svc)(context.Background(), nil, listAlertsInput{})
	require.NoError(t, err)
	assert.True(t, spy.activeOnlyCalled, "nil active_only must default to ListActiveAlerts")
	assert.False(t, spy.listAlertsCalled)
}

func TestListAlertsHandler_ExplicitActiveOnlyTrue(t *testing.T) {
	spy := newMCPListAlertsSpy()
	svc := &Services{Alerts: spy, Logger: slog.Default(), Version: "test"}
	activeOnly := true

	_, _, err := listAlertsHandler(svc)(context.Background(), nil, listAlertsInput{ActiveOnly: &activeOnly})
	require.NoError(t, err)
	assert.True(t, spy.activeOnlyCalled)
	assert.False(t, spy.listAlertsCalled)
}

func TestListAlertsHandler_ActiveOnlyFalse_ReturnsAllStatuses(t *testing.T) {
	spy := newMCPListAlertsSpy()
	svc := &Services{Alerts: spy, Logger: slog.Default(), Version: "test"}
	activeOnly := false

	_, _, err := listAlertsHandler(svc)(context.Background(), nil, listAlertsInput{ActiveOnly: &activeOnly})
	require.NoError(t, err)
	assert.False(t, spy.activeOnlyCalled, "active_only:false must not call ListActiveAlerts")
	assert.True(t, spy.listAlertsCalled)
	assert.Equal(t, 100, spy.listAlertsOpts.Limit)
}

func TestGetContainerLogsInput_Defaults(t *testing.T) {
	input := getContainerLogsInput{}
	assert.Equal(t, 0, input.Lines, "Lines should default to zero (handler applies default of 100)")
	assert.False(t, input.Timestamps, "Timestamps should default to false")
}

func TestGetTopConsumersInput_Validation(t *testing.T) {
	for _, metric := range []string{"cpu", "memory"} {
		input := getTopConsumersInput{Metric: metric}
		assert.Contains(t, []string{"cpu", "memory"}, input.Metric)
	}

	input := getTopConsumersInput{Metric: "disk"}
	assert.NotEqual(t, "cpu", input.Metric)
	assert.NotEqual(t, "memory", input.Metric)
}

func TestGetEndpointHistoryInput_DefaultLimit(t *testing.T) {
	input := getEndpointHistoryInput{EndpointID: "5"}
	assert.Equal(t, "5", input.EndpointID)
	assert.Equal(t, 0, input.Limit, "Limit should default to zero (handler applies default of 50)")
}

// --- get_top_consumers: the history cap applies here too (FR-015a) ---

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = original })
}

// This surface has no interface in front of it, so a cap enforced only in the
// frontend would not exist here at all.
func TestGetTopConsumers_RefusesAWindowAboveTheCap(t *testing.T) {
	pinEdition(t, extension.Community)

	// Services is deliberately empty: a refused window must never reach the
	// store, so a nil resource service is enough to prove it.
	handler := getTopConsumersHandler(&Services{})
	result, _, err := handler(context.Background(), nil, getTopConsumersInput{Metric: "cpu", Period: "30d"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "edition_required")
	assert.Contains(t, text, `"required_edition":"personal"`)
	assert.Contains(t, text, `"window":"30d"`)
	assert.Contains(t, text, `"max_window":"7d"`)
}

func TestGetTopConsumers_RefusesAWindowTheProductDoesNotKnow(t *testing.T) {
	pinEdition(t, extension.Pro)

	handler := getTopConsumersHandler(&Services{})
	result, _, err := handler(context.Background(), nil, getTopConsumersInput{Metric: "cpu", Period: "2d"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "invalid input")
	assert.NotContains(t, text, "edition_required", "a bad request is not an edition question")
}

// The live ranking (period absent or "current") never reaches the store: it is
// answered by resource.Service.TopConsumersNow, covered in that package. Before
// this feature, "current" was handed to the store, which has never known that
// value, so the tool failed every time it was called without a period.
