// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C8, on the MCP channel tools.

var (
	mcpInsertedAt = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	mcpUpdatedAt  = time.Date(2026, 10, 21, 11, 30, 0, 0, time.UTC)
)

// datedChannelStore stamps the rows with dates the handlers cannot know, so a response can only
// carry them by reading the row back.
type datedChannelStore struct {
	*mcpLiveChannelStore
}

func (s *datedChannelStore) InsertChannel(ctx context.Context, ch *alert.NotificationChannel) (string, error) {
	id, err := s.mcpLiveChannelStore.InsertChannel(ctx, ch)
	if err == nil {
		s.channels[id].CreatedAt = mcpInsertedAt
		s.channels[id].UpdatedAt = mcpInsertedAt
	}
	return id, err
}

func (s *datedChannelStore) UpdateChannel(ctx context.Context, ch *alert.NotificationChannel) error {
	if err := s.mcpLiveChannelStore.UpdateChannel(ctx, ch); err != nil {
		return err
	}
	s.channels[ch.ID].CreatedAt = mcpInsertedAt
	s.channels[ch.ID].UpdatedAt = mcpUpdatedAt
	return nil
}

func buildSecretChannelServices(t *testing.T) (*Services, *datedChannelStore) {
	t.Helper()
	withEdition(t, extension.Pro)
	svc, cs, _ := buildChannelServices()
	dated := &datedChannelStore{mcpLiveChannelStore: cs}
	svc.Channels = dated
	return svc, dated
}

func createOK(t *testing.T, svc *Services, in createChannelInput) (string, map[string]any, string) {
	t.Helper()
	result, _, err := createChannelHandler(svc)(context.Background(), nil, in)
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	text := textFromContent(t, result.Content)
	m := parseChannelResult(t, text)
	id, _ := m["id"].(string)
	require.NotEmpty(t, id)
	return id, m, text
}

func str(s string) *string { return &s }

// US3 scenario 1, FR-020 on MCP: malformed headers are refused with field=headers, and nothing is written.
func TestMCPChannels_MalformedHeadersAreRefused(t *testing.T) {
	malformed := []string{"x", "[1]", `{"a":1}`, `{"a":{"b":"c"}}`, `{"":"v"}`}

	for _, headers := range malformed {
		t.Run("create "+headers, func(t *testing.T) {
			svc, cs := buildSecretChannelServices(t)
			result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
				Name: "bad", URL: "https://hooks.example.com/x", Headers: headers,
			})
			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Contains(t, textFromContent(t, result.Content), "field=headers")
			assert.Empty(t, cs.channels)
		})
	}

	for _, headers := range malformed {
		t.Run("update "+headers, func(t *testing.T) {
			svc, cs := buildSecretChannelServices(t)
			id, _, _ := createOK(t, svc, createChannelInput{Name: "ops", URL: "https://hooks.example.com/x", Headers: `{"Authorization":"Bearer s3cr3t"}`})

			result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Headers: str(headers)})
			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Contains(t, textFromContent(t, result.Content), "field=headers")
			assert.Equal(t, `{"Authorization":"Bearer s3cr3t"}`, cs.channels[id].Headers)
		})
	}
}

// US3 scenario 2 and 8, FR-021, SC-004: create_channel, list_channels, get_channel and update_channel mask the values.
func TestMCPChannels_HeaderValuesAreMaskedOnEveryTool(t *testing.T) {
	svc, cs := buildSecretChannelServices(t)
	id, created, text := createOK(t, svc, createChannelInput{
		Name: "ops", URL: "https://hooks.example.com/x", Headers: `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`,
	})
	masked := `{"Authorization":"***","X-Trace":"***"}`
	assert.Equal(t, masked, created["headers"])
	assert.Equal(t, true, created["has_headers"])
	assert.NotContains(t, text, "s3cr3t")

	result, _, err := listChannelsHandler(svc)(context.Background(), nil, listChannelsInput{})
	require.NoError(t, err)
	listText := textFromContent(t, result.Content)
	assert.NotContains(t, listText, "s3cr3t")
	assert.Contains(t, listText, `\"Authorization\":\"***\"`)

	result, _, err = getChannelHandler(svc)(context.Background(), nil, getChannelInput{ID: id})
	require.NoError(t, err)
	getText := textFromContent(t, result.Content)
	assert.NotContains(t, getText, "s3cr3t")
	ch, ok := parseChannelResult(t, getText)["channel"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, masked, ch["headers"])
	assert.Equal(t, true, ch["has_headers"])

	result, _, err = updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Name: str("ops-2")})
	require.NoError(t, err)
	require.False(t, result.IsError)
	updText := textFromContent(t, result.Content)
	assert.NotContains(t, updText, "s3cr3t")
	assert.Equal(t, masked, parseChannelResult(t, updText)["headers"])

	assert.Equal(t, `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`, cs.channels[id].Headers, "the row keeps the real values")
}

// US3 scenario 3: a channel without headers says so and carries no headers key.
func TestMCPChannels_NoHeadersMeansNoHeadersKey(t *testing.T) {
	svc, _ := buildSecretChannelServices(t)
	_, created, _ := createOK(t, svc, createChannelInput{Name: "plain", URL: "https://hooks.example.com/x"})

	assert.Equal(t, false, created["has_headers"])
	_, present := created["headers"]
	assert.False(t, present)
}

// US3 scenario 4, FR-023 on MCP: platform URLs are cut after the host.
func TestMCPChannels_PlatformURLsAreTruncated(t *testing.T) {
	for _, tc := range []struct {
		typ, url, want string
		secret         bool
	}{
		{"slack", "https://hooks.slack.com/services/T000/B000/XXXXsecret", "https://hooks.slack.com", true},
		{"discord", "https://discord.com/api/webhooks/1234/abcdTOKEN", "https://discord.com", true},
		{"teams", "https://acme.webhook.office.com/webhookb2/guid/IncomingWebhook/id/key", "https://acme.webhook.office.com", true},
		{"webhook", "https://hooks.example.com/path/kept", "https://hooks.example.com/path/kept", false},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			svc, cs := buildSecretChannelServices(t)
			id, created, _ := createOK(t, svc, createChannelInput{Name: "n", Type: tc.typ, URL: tc.url})
			assert.Equal(t, tc.want, created["url"])
			assert.Equal(t, tc.secret, created["has_url_secret"])

			result, _, err := getChannelHandler(svc)(context.Background(), nil, getChannelInput{ID: id})
			require.NoError(t, err)
			text := textFromContent(t, result.Content)
			if tc.secret {
				assert.NotContains(t, text, "XXXXsecret")
				assert.NotContains(t, text, "TOKEN")
				assert.NotContains(t, text, "IncomingWebhook")
			}
			ch := parseChannelResult(t, text)["channel"].(map[string]any)
			assert.Equal(t, tc.want, ch["url"])
			assert.Equal(t, tc.secret, ch["has_url_secret"])
			assert.Equal(t, tc.url, cs.channels[id].URL)
		})
	}
}

// US3 scenario 5, FR-022, FR-023 on MCP: omitted headers and url are kept; {} clears the headers.
func TestMCPChannels_UpdateKeepsOmittedHeadersAndURL(t *testing.T) {
	svc, cs := buildSecretChannelServices(t)
	id, _, _ := createOK(t, svc, createChannelInput{
		Name: "ops", Type: "slack", URL: "https://hooks.slack.com/services/T000/B000/XXXXsecret", Headers: `{"X-Trace":"1"}`,
	})

	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Name: str("ops-2")})
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	assert.Equal(t, `{"X-Trace":"1"}`, cs.channels[id].Headers)
	assert.Equal(t, "https://hooks.slack.com/services/T000/B000/XXXXsecret", cs.channels[id].URL)
	renamed := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, true, renamed["has_headers"], "the kept headers are still reported")
	assert.Equal(t, `{"X-Trace":"***"}`, renamed["headers"])
	assert.Equal(t, "https://hooks.slack.com", renamed["url"])

	result, _, err = updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Headers: str("{}")})
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	assert.Equal(t, false, parseChannelResult(t, textFromContent(t, result.Content))["has_headers"])
	assert.Contains(t, []string{"", "{}"}, cs.channels[id].Headers, "headers: {} clears the stored headers")
}

// US3 scenario 7, FR-024 on MCP: create_channel and update_channel answer with the stored row, dates included.
func TestMCPChannels_ResponsesCarryTheStoredDates(t *testing.T) {
	svc, _ := buildSecretChannelServices(t)
	id, created, _ := createOK(t, svc, createChannelInput{Name: "ops", URL: "https://hooks.example.com/x"})
	assert.Equal(t, mcpInsertedAt.Format(time.RFC3339), created["created_at"])
	assert.Equal(t, mcpInsertedAt.Format(time.RFC3339), created["updated_at"])

	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Name: str("ops-2")})
	require.NoError(t, err)
	require.False(t, result.IsError)
	updated := parseChannelResult(t, textFromContent(t, result.Content))
	assert.Equal(t, mcpInsertedAt.Format(time.RFC3339), updated["created_at"])
	assert.Equal(t, mcpUpdatedAt.Format(time.RFC3339), updated["updated_at"])
}
