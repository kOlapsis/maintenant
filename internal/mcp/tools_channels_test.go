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
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mutable channel store ---

type mcpLiveChannelStore struct {
	channels map[string]*alert.NotificationChannel
	nextID   int64
}

func newMCPLiveChannelStore() *mcpLiveChannelStore {
	return &mcpLiveChannelStore{channels: map[string]*alert.NotificationChannel{}}
}

func (m *mcpLiveChannelStore) InsertChannel(_ context.Context, ch *alert.NotificationChannel) (string, error) {
	m.nextID++
	id := strconv.FormatInt(m.nextID, 10)
	cp := *ch
	cp.ID = id
	cp.CreatedAt = time.Now()
	m.channels[id] = &cp
	return id, nil
}
func (m *mcpLiveChannelStore) GetChannel(_ context.Context, id string) (*alert.NotificationChannel, error) {
	ch, ok := m.channels[id]
	if !ok {
		return nil, nil
	}
	cp := *ch
	return &cp, nil
}
func (m *mcpLiveChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	out := make([]*alert.NotificationChannel, 0, len(m.channels))
	for _, ch := range m.channels {
		out = append(out, ch)
	}
	return out, nil
}
func (m *mcpLiveChannelStore) UpdateChannel(_ context.Context, ch *alert.NotificationChannel) error {
	cp := *ch
	m.channels[ch.ID] = &cp
	return nil
}
func (m *mcpLiveChannelStore) DeleteChannel(_ context.Context, id string) error {
	delete(m.channels, id)
	return nil
}
func (m *mcpLiveChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "healthy", nil
}
func (m *mcpLiveChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "1", nil
}
func (m *mcpLiveChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *mcpLiveChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// --- notifier mock ---

type mcpChannelTester struct {
	code int
	err  error
	sent []string
}

func (m *mcpChannelTester) SendTestWebhook(_ context.Context, ch *alert.NotificationChannel) (int, error) {
	m.sent = append(m.sent, ch.ID)
	return m.code, m.err
}

// --- helpers ---

func buildChannelServices() (*Services, *mcpLiveChannelStore, *[]string) {
	cs := newMCPLiveChannelStore()
	var events []string
	svc := &Services{
		Channels: cs,
		// The SSRF guard resolves DNS; the destination rules have their own
		// tests in internal/ssrf, so keep these off the network.
		AllowPrivateWebhooks: true,
		Broadcast: func(eventType string, _ any) {
			events = append(events, eventType)
		},
	}
	return svc, cs, &events
}

func parseChannelResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &m))
	return m
}

// --- list_channels ---

func TestListChannelsHandler_Empty(t *testing.T) {
	svc, _, _ := buildChannelServices()

	result, _, err := listChannelsHandler(svc)(context.Background(), nil, listChannelsInput{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	m := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Empty(t, m["channels"])
}

func TestListChannelsHandler_NeverLeaksTheSecret(t *testing.T) {
	withEdition(t, extension.Personal)
	svc, cs, _ := buildChannelServices()
	_, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "telegram", Type: "telegram", URL: "123", Secret: "123456:SUPER-SECRET-TOKEN",
	})
	require.NoError(t, err)

	result, _, err := listChannelsHandler(svc)(context.Background(), nil, listChannelsInput{})
	require.NoError(t, err)
	assert.NotContains(t, textFromContent(t, result.Content), "SUPER-SECRET-TOKEN")
}

// --- get_channel ---

func TestGetChannelHandler_NotFound(t *testing.T) {
	svc, _, _ := buildChannelServices()

	result, _, err := getChannelHandler(svc)(context.Background(), nil, getChannelInput{ID: "404"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestGetChannelHandler_ReportsHealthAndTriggers(t *testing.T) {
	svc, cs, _ := buildChannelServices()
	ts := newMCPTriggerStore()
	svc.Triggers = ts
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "ops", Type: "webhook"})
	require.NoError(t, err)

	result, _, err := getChannelHandler(svc)(context.Background(), nil, getChannelInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)

	m := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, "healthy", m["health"])
	assert.Equal(t, []any{}, m["triggers"])
}

// --- create_channel ---

func TestCreateChannelHandler_DefaultsToWebhook(t *testing.T) {
	svc, cs, events := buildChannelServices()

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
		Name: "ops", URL: "https://hooks.example.com/abc",
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	m := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, "webhook", m["type"])
	assert.Equal(t, true, m["enabled"], "a channel created without an explicit flag is live")
	assert.Len(t, cs.channels, 1)
	assert.Equal(t, []string{event.ChannelCreated}, *events)
}

func TestCreateChannelHandler_RequiresNameAndURL(t *testing.T) {
	svc, _, _ := buildChannelServices()

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{URL: "https://x.example.com"})
	require.NoError(t, err)
	assert.True(t, result.IsError)

	result, _, err = createChannelHandler(svc)(context.Background(), nil, createChannelInput{Name: "ops"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestCreateChannelHandler_RejectsInvalidEmail(t *testing.T) {
	withEdition(t, extension.Personal)
	svc, _, _ := buildChannelServices()

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
		Name: "mail", Type: "email", URL: "not-an-address",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestCreateChannelHandler_RefusesGatedTypeOnCommunity(t *testing.T) {
	withEdition(t, extension.Community)
	svc, cs, events := buildChannelServices()

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
		Name: "slack", Type: "slack", URL: "https://hooks.slack.com/services/x",
	})
	require.NoError(t, err)
	require.True(t, result.IsError)

	var payload struct {
		Error           string `json:"error"`
		Feature         string `json:"feature"`
		RequiredEdition string `json:"required_edition"`
	}
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &payload))
	assert.Equal(t, "edition_required", payload.Error)
	assert.Equal(t, string(extension.CapSlack), payload.Feature)
	assert.Equal(t, string(extension.Pro), payload.RequiredEdition)
	assert.Empty(t, cs.channels)
	assert.Empty(t, *events)
}

func TestCreateChannelHandler_TelegramNeedsAValidToken(t *testing.T) {
	withEdition(t, extension.Personal)
	svc, _, _ := buildChannelServices()

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
		Name: "tg", Type: "telegram", URL: "123456789", Secret: "not-a-token",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- update_channel ---

func TestUpdateChannelHandler_KeepsTheStoredSecret(t *testing.T) {
	withEdition(t, extension.Personal)
	svc, cs, _ := buildChannelServices()
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "tg", Type: "telegram", URL: "123", Secret: "123456:kept",
	})
	require.NoError(t, err)

	name := "renamed"
	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Name: &name})
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, "renamed", cs.channels[id].Name)
	assert.Equal(t, "123456:kept", cs.channels[id].Secret)
}

func TestUpdateChannelHandler_RefusesToClearTheSecret(t *testing.T) {
	withEdition(t, extension.Personal)
	svc, cs, _ := buildChannelServices()
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "tg", Type: "telegram", URL: "123", Secret: "123456:kept",
	})
	require.NoError(t, err)

	empty := ""
	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Secret: &empty})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Equal(t, "123456:kept", cs.channels[id].Secret)
}

// A downgraded instance must still be able to switch a gated channel off,
// otherwise deleting it is the only way to stop the deliveries.
func TestUpdateChannelHandler_DisablingAGatedChannelStaysAllowed(t *testing.T) {
	svc, cs, _ := buildChannelServices()
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "slack", Type: "slack", URL: "https://hooks.slack.com/services/x", Enabled: true,
	})
	require.NoError(t, err)

	withEdition(t, extension.Community)
	off := false
	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Enabled: &off})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.False(t, cs.channels[id].Enabled)

	on := true
	result, _, err = updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Enabled: &on})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestUpdateChannelHandler_NotFound(t *testing.T) {
	svc, _, _ := buildChannelServices()

	name := "x"
	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: "404", Name: &name})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- delete_channel ---

func TestDeleteChannelHandler(t *testing.T) {
	svc, cs, events := buildChannelServices()
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "ops", Type: "webhook"})
	require.NoError(t, err)

	result, _, err := deleteChannelHandler(svc)(context.Background(), nil, deleteChannelInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Empty(t, cs.channels)
	assert.Equal(t, []string{event.ChannelDeleted}, *events)

	result, _, err = deleteChannelHandler(svc)(context.Background(), nil, deleteChannelInput{ID: id})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- test_channel ---

func TestTestChannelHandler_Delivered(t *testing.T) {
	svc, cs, _ := buildChannelServices()
	svc.ChannelTester = &mcpChannelTester{code: 204}
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "ops", Type: "webhook"})
	require.NoError(t, err)

	result, _, err := testChannelHandler(svc)(context.Background(), nil, testChannelInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)

	m := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, "delivered", m["status"])
	assert.Equal(t, float64(204), m["response_code"])
}

func TestTestChannelHandler_ReportsTheFailure(t *testing.T) {
	svc, cs, _ := buildChannelServices()
	svc.ChannelTester = &mcpChannelTester{err: errors.New("connection refused")}
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "ops", Type: "webhook"})
	require.NoError(t, err)

	result, _, err := testChannelHandler(svc)(context.Background(), nil, testChannelInput{ID: id})
	require.NoError(t, err)
	require.False(t, result.IsError)

	m := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, "failed", m["status"])
	assert.Equal(t, "connection refused", m["error"])
}

func TestTestChannelHandler_RefusesAGatedTypeWithoutSending(t *testing.T) {
	svc, cs, _ := buildChannelServices()
	tester := &mcpChannelTester{code: 200}
	svc.ChannelTester = tester
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "slack", Type: "slack"})
	require.NoError(t, err)

	withEdition(t, extension.Community)
	result, _, err := testChannelHandler(svc)(context.Background(), nil, testChannelInput{ID: id})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Empty(t, tester.sent)
}

// The create_channel description names editions from the registry, so it can
// never advertise a tier the gate does not enforce.
func TestGatedChannelTypes_ReadsTheRegistry(t *testing.T) {
	assert.Equal(t,
		"email needs Personal, slack needs Pro, teams needs Pro, telegram needs Personal",
		gatedChannelTypes())
}
