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
	"strconv"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock trigger store
// ---------------------------------------------------------------------------

type mcpTriggerStore struct {
	triggers map[string]*alert.AlertTrigger
	nextID   int64
}

func newMCPTriggerStore() *mcpTriggerStore {
	return &mcpTriggerStore{triggers: map[string]*alert.AlertTrigger{}}
}

func (m *mcpTriggerStore) InsertTrigger(_ context.Context, t *alert.AlertTrigger) (string, error) {
	m.nextID++
	t.ID = strconv.FormatInt(m.nextID, 10)
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	cp := *t
	m.triggers[t.ID] = &cp
	return t.ID, nil
}
func (m *mcpTriggerStore) GetTrigger(_ context.Context, id string) (*alert.AlertTrigger, error) {
	return m.triggers[id], nil
}
func (m *mcpTriggerStore) ListTriggers(_ context.Context) ([]*alert.AlertTrigger, error) {
	out := make([]*alert.AlertTrigger, 0, len(m.triggers))
	for _, t := range m.triggers {
		out = append(out, t)
	}
	return out, nil
}
func (m *mcpTriggerStore) ListEnabledTriggers(_ context.Context) ([]*alert.AlertTrigger, error) {
	return nil, nil
}
func (m *mcpTriggerStore) UpdateTrigger(_ context.Context, t *alert.AlertTrigger) error {
	cp := *t
	m.triggers[t.ID] = &cp
	return nil
}
func (m *mcpTriggerStore) DeleteTrigger(_ context.Context, id string) error {
	delete(m.triggers, id)
	return nil
}
func (m *mcpTriggerStore) SetChannels(_ context.Context, _ string, _ []string) error { return nil }
func (m *mcpTriggerStore) ListChannelsForTrigger(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}
func (m *mcpTriggerStore) ListTriggersForChannel(_ context.Context, _ string) ([]*alert.AlertTrigger, error) {
	return nil, nil
}

// mcpNilChannelStore always returns nil (channel not found), used for validation tests.
type mcpNilChannelStore struct{}

func (m *mcpNilChannelStore) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (string, error) {
	return "", nil
}
func (m *mcpNilChannelStore) GetChannel(_ context.Context, _ string) (*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *mcpNilChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *mcpNilChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (m *mcpNilChannelStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (m *mcpNilChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (m *mcpNilChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "", nil
}
func (m *mcpNilChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *mcpNilChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func buildTriggerServices() (*Services, *mcpTriggerStore) {
	ts := newMCPTriggerStore()
	return &Services{Triggers: ts, Channels: &mcpChannelStore{}}, ts
}

// parseTriggersResult unmarshals the JSON content into a map and returns triggers.
func parseTriggersResult(t *testing.T, text string) []any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &m))
	triggers, ok := m["triggers"].([]any)
	if !ok {
		return []any{}
	}
	return triggers
}

// ---------------------------------------------------------------------------
// list_triggers
// ---------------------------------------------------------------------------

func TestListTriggersHandler_Empty(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := listTriggersHandler(svc)

	result, _, err := handler(context.Background(), nil, listTriggersInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	triggers := parseTriggersResult(t, textFromContent(t, result.Content))
	assert.Empty(t, triggers)
}

func TestListTriggersHandler_ReturnsTriggers(t *testing.T) {
	svc, ts := buildTriggerServices()
	_, err := ts.InsertTrigger(context.Background(), &alert.AlertTrigger{
		Name: "CritContainers", Enabled: true, ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)

	handler := listTriggersHandler(svc)
	result, _, err := handler(context.Background(), nil, listTriggersInput{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	triggers := parseTriggersResult(t, textFromContent(t, result.Content))
	assert.Len(t, triggers, 1)
}

// ---------------------------------------------------------------------------
// get_trigger
// ---------------------------------------------------------------------------

func TestGetTriggerHandler_Found(t *testing.T) {
	svc, ts := buildTriggerServices()
	id, err := ts.InsertTrigger(context.Background(), &alert.AlertTrigger{
		Name: "FindMe", Enabled: true, ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)

	handler := getTriggerHandler(svc)
	result, _, err := handler(context.Background(), nil, getTriggerInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "FindMe")
}

func TestGetTriggerHandler_NotFound(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := getTriggerHandler(svc)
	result, _, err := handler(context.Background(), nil, getTriggerInput{ID: "9999"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not found")
}

// ---------------------------------------------------------------------------
// create_trigger
// ---------------------------------------------------------------------------

func TestCreateTriggerHandler_Happy(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:       "AlertAll",
		Enabled:    true,
		ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "AlertAll")
}

func TestCreateTriggerHandler_EmptyName(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:       "",
		ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "name")
}

func TestCreateTriggerHandler_EmptyChannelIDs(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:       "NoChannel",
		ChannelIDs: []string{},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "channel")
}

func TestCreateTriggerHandler_ChannelNotFound(t *testing.T) {
	ts := newMCPTriggerStore()
	svc := &Services{Triggers: ts, Channels: &mcpNilChannelStore{}}
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:       "OrphanTrigger",
		ChannelIDs: []string{"999"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "999")
}

func TestCreateTriggerHandler_FilterScopes_CommunityBlocked(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:         "ScopedTrigger",
		FilterScopes: "container:42",
		ChannelIDs:   []string{"1"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestCreateTriggerHandler_FilterTags_CommunityBlocked(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:       "TaggedTrigger",
		FilterTags: "prod",
		ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestCreateTriggerHandler_FilterScopes_ProAllowed(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc, _ := buildTriggerServices()
	handler := createTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, triggerInput{
		Name:         "ProTrigger",
		FilterScopes: "container:7",
		ChannelIDs:   []string{"1"},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
}

// ---------------------------------------------------------------------------
// update_trigger
// ---------------------------------------------------------------------------

func TestUpdateTriggerHandler_Happy(t *testing.T) {
	svc, ts := buildTriggerServices()
	id, err := ts.InsertTrigger(context.Background(), &alert.AlertTrigger{
		Name: "BeforeUpdate", Enabled: true, ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)

	handler := updateTriggerHandler(svc)
	result, _, err := handler(context.Background(), nil, updateTriggerInputWithID{
		ID: id,
		triggerInput: triggerInput{
			Name:       "AfterUpdate",
			Enabled:    true,
			ChannelIDs: []string{"1"},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "AfterUpdate")
}

func TestUpdateTriggerHandler_NotFound(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := updateTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, updateTriggerInputWithID{
		ID:           "9999",
		triggerInput: triggerInput{Name: "Ghost", ChannelIDs: []string{"1"}},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not found")
}

// ---------------------------------------------------------------------------
// delete_trigger
// ---------------------------------------------------------------------------

func TestDeleteTriggerHandler_Happy(t *testing.T) {
	svc, ts := buildTriggerServices()
	id, err := ts.InsertTrigger(context.Background(), &alert.AlertTrigger{
		Name: "ToRemove", Enabled: true, ChannelIDs: []string{"1"},
	})
	require.NoError(t, err)

	handler := deleteTriggerHandler(svc)
	result, _, err := handler(context.Background(), nil, deleteTriggerInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "deleted")
}

func TestDeleteTriggerHandler_NotFound(t *testing.T) {
	svc, _ := buildTriggerServices()
	handler := deleteTriggerHandler(svc)

	result, _, err := handler(context.Background(), nil, deleteTriggerInput{ID: "9999"})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not found")
}
