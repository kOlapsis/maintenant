package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ssrf"
	"github.com/kolapsis/maintenant/internal/store"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerChannelTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_channels",
		Description: "List notification channels. Channels are silent on their own: an alert reaches one only through an alert trigger or an escalation level. Secrets are never returned.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listChannelsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_channel",
		Description: "Get a single notification channel by ID, with its delivery health and the triggers routing to it.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getChannelHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_channel",
		Description: "Create a notification channel. The webhook type is open in every edition; " + gatedChannelTypes() + ".",
	}, createChannelHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_channel",
		Description: "Update a notification channel. Omitted fields keep their stored value; a stored secret is never cleared this way.",
	}, updateChannelHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "delete_channel",
		Description: "Delete a notification channel. Triggers and escalation levels pointing at it lose that destination.",
	}, deleteChannelHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "test_channel",
		Description: "Send a test notification through a channel and report whether it was delivered.",
	}, testChannelHandler(svc))
}

// gatedChannelTypes renders the edition each gated channel type needs, read
// from the registry so a description cannot name a tier the gate does not
// enforce.
func gatedChannelTypes() string {
	types := []string{"email", "slack", "teams", "telegram"}
	parts := make([]string, 0, len(types))
	for _, t := range types {
		c, _ := extension.ChannelCapability(t)
		parts = append(parts, t+" needs "+titleEdition(extension.MinEdition(c)))
	}
	return strings.Join(parts, ", ")
}

type listChannelsInput struct{}

type getChannelInput struct {
	ID string `json:"id" jsonschema:"Channel ID"`
}

type createChannelInput struct {
	Name    string `json:"name" jsonschema:"Channel name, unique across channels"`
	Type    string `json:"type" jsonschema:"Channel type: webhook, email, telegram, slack or teams. Defaults to webhook."`
	URL     string `json:"url" jsonschema:"Destination: an HTTPS webhook URL, an email address, or a Telegram chat id"`
	Headers string `json:"headers,omitempty" jsonschema:"Extra HTTP headers as a JSON object, webhook types only"`
	Secret  string `json:"secret,omitempty" jsonschema:"Channel credential, a Telegram bot token today. Write-only: it is never read back."`
	Config  string `json:"config,omitempty" jsonschema:"Non-secret per-type settings as a JSON object, e.g. {\"thread_id\":\"42\"} for Telegram"`
	Enabled *bool  `json:"enabled,omitempty" jsonschema:"Whether the channel accepts deliveries, default true"`
}

type updateChannelInput struct {
	ID      string  `json:"id" jsonschema:"Channel ID to update"`
	Name    *string `json:"name,omitempty" jsonschema:"New channel name"`
	Type    *string `json:"type,omitempty" jsonschema:"New channel type"`
	URL     *string `json:"url,omitempty" jsonschema:"New destination"`
	Headers *string `json:"headers,omitempty" jsonschema:"New extra HTTP headers as a JSON object"`
	Secret  *string `json:"secret,omitempty" jsonschema:"New credential. Omit to keep the stored one; it cannot be set to empty."`
	Config  *string `json:"config,omitempty" jsonschema:"New non-secret per-type settings as a JSON object"`
	Enabled *bool   `json:"enabled,omitempty" jsonschema:"Whether the channel accepts deliveries"`
}

type deleteChannelInput struct {
	ID string `json:"id" jsonschema:"Channel ID to delete"`
}

type testChannelInput struct {
	ID string `json:"id" jsonschema:"Channel ID to test"`
}

// refuseChannelType is the MCP half of the REST refuseChannelCapability: same
// registry, same decision, named per channel type so the refusal says which
// edition opens that one.
func refuseChannelType(chType string) (*gomcp.CallToolResult, any, error) {
	c, gated := extension.ChannelCapability(chType)
	if !gated || extension.Allows(c) {
		return nil, nil, nil
	}
	required := extension.MinEdition(c)
	msg := fmt.Sprintf(
		`{"error":"edition_required","feature":%q,"required_edition":%q,"message":"The %s channel requires the %s edition of Maintenant."}`,
		string(c), string(required), chType, titleEdition(required),
	)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

func validateChannelDestination(ctx context.Context, svc *Services, chType, rawURL string) error {
	switch chType {
	case "telegram":
		return alert.ValidateChatID(rawURL)
	case "email":
		if _, err := mail.ParseAddress(rawURL); err != nil {
			return errors.New("invalid email address")
		}
		return nil
	}
	if svc.AllowPrivateWebhooks {
		return nil
	}
	return ssrf.ValidateURL(ctx, rawURL)
}

func validateChannelCredentials(chType, secret, config string) error {
	if chType != "telegram" {
		return nil
	}
	if err := alert.ValidateBotToken(secret); err != nil {
		return err
	}
	cfg, err := alert.ParseTelegramConfig(config)
	if err != nil {
		return errors.New("config must be a JSON object")
	}
	return alert.ValidateThreadID(cfg.ThreadID)
}

func normalizeConfig(config string) string {
	config = strings.TrimSpace(config)
	if config == "null" {
		return ""
	}
	return config
}

func (s *Services) broadcast(eventType string, data any) {
	if s.Broadcast != nil {
		s.Broadcast(eventType, data)
	}
}

func listChannelsHandler(svc *Services) gomcp.ToolHandlerFor[listChannelsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listChannelsInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		channels, err := svc.Channels.ListChannels(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list channels: %w", err)
		}
		if channels == nil {
			channels = []*alert.NotificationChannel{}
		}
		return jsonResult(map[string]any{"channels": channels})
	}
}

func getChannelHandler(svc *Services) gomcp.ToolHandlerFor[getChannelInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getChannelInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		ch, err := svc.Channels.GetChannel(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("get channel: %w", err)
		}
		if ch == nil {
			return errResult("channel not found")
		}

		out := map[string]any{"channel": ch}
		if health, err := svc.Channels.GetChannelHealth(ctx, ch.ID); err == nil {
			out["health"] = health
		}
		if svc.Triggers != nil {
			triggers, err := svc.Triggers.ListTriggersForChannel(ctx, ch.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("get channel: triggers: %w", err)
			}
			if triggers == nil {
				triggers = []*alert.AlertTrigger{}
			}
			out["triggers"] = triggers
		}
		return jsonResult(out)
	}
}

func createChannelHandler(svc *Services) gomcp.ToolHandlerFor[createChannelInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createChannelInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		if input.Type == "" {
			input.Type = "webhook"
		}
		if r, v, err := refuseChannelType(input.Type); r != nil {
			return r, v, err
		}
		if input.Name == "" {
			return errResult("field=name: required")
		}
		if input.URL == "" {
			return errResult("field=url: required")
		}
		if err := validateChannelDestination(ctx, svc, input.Type, input.URL); err != nil {
			return errResult("field=url: " + err.Error())
		}
		config := normalizeConfig(input.Config)
		if err := validateChannelCredentials(input.Type, input.Secret, config); err != nil {
			return errResult(err.Error())
		}

		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}

		ch := &alert.NotificationChannel{
			Name:    input.Name,
			Type:    input.Type,
			URL:     input.URL,
			Headers: input.Headers,
			Secret:  input.Secret,
			Config:  config,
			Enabled: enabled,
		}
		id, err := svc.Channels.InsertChannel(ctx, ch)
		if err != nil {
			if store.IsUniqueViolation(err) {
				return errResult("a channel with this name already exists")
			}
			return nil, nil, fmt.Errorf("create channel: %w", err)
		}
		ch.ID = id
		ch.HasSecret = ch.Secret != ""

		svc.broadcast(event.ChannelCreated, ch)
		return jsonResult(ch)
	}
}

func updateChannelHandler(svc *Services) gomcp.ToolHandlerFor[updateChannelInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input updateChannelInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		ch, err := svc.Channels.GetChannel(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("update channel: lookup: %w", err)
		}
		if ch == nil {
			return errResult("channel not found")
		}

		// Switching a channel off never needs the edition that opens its type:
		// a downgraded instance must still be able to silence it.
		extends := input.Name != nil || input.Type != nil || input.URL != nil ||
			input.Headers != nil || input.Secret != nil || input.Config != nil ||
			(input.Enabled != nil && *input.Enabled)
		if extends {
			if r, v, err := refuseChannelType(ch.Type); r != nil {
				return r, v, err
			}
			if input.Type != nil {
				if r, v, err := refuseChannelType(*input.Type); r != nil {
					return r, v, err
				}
			}
		}

		if input.Name != nil {
			ch.Name = *input.Name
		}
		if input.Type != nil {
			ch.Type = *input.Type
		}
		if input.URL != nil {
			ch.URL = *input.URL
		}
		if input.Headers != nil {
			ch.Headers = *input.Headers
		}
		if input.Secret != nil {
			if *input.Secret == "" {
				return errResult("field=secret: cannot be cleared; delete the channel instead")
			}
			ch.Secret = *input.Secret
		}
		if input.Config != nil {
			ch.Config = normalizeConfig(*input.Config)
		}
		if input.Enabled != nil {
			ch.Enabled = *input.Enabled
		}

		if input.Secret != nil || input.Config != nil {
			if err := validateChannelCredentials(ch.Type, ch.Secret, ch.Config); err != nil {
				return errResult(err.Error())
			}
		}
		if input.URL != nil || input.Type != nil {
			if err := validateChannelDestination(ctx, svc, ch.Type, ch.URL); err != nil {
				return errResult("field=url: " + err.Error())
			}
		}

		if err := svc.Channels.UpdateChannel(ctx, ch); err != nil {
			return nil, nil, fmt.Errorf("update channel: %w", err)
		}
		ch.HasSecret = ch.Secret != ""

		svc.broadcast(event.ChannelUpdated, ch)
		return jsonResult(ch)
	}
}

func deleteChannelHandler(svc *Services) gomcp.ToolHandlerFor[deleteChannelInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input deleteChannelInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		ch, err := svc.Channels.GetChannel(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("delete channel: lookup: %w", err)
		}
		if ch == nil {
			return errResult("channel not found")
		}
		if err := svc.Channels.DeleteChannel(ctx, input.ID); err != nil {
			return nil, nil, fmt.Errorf("delete channel: %w", err)
		}

		svc.broadcast(event.ChannelDeleted, map[string]any{"id": input.ID})
		return jsonResult(map[string]any{"deleted": true, "id": input.ID})
	}
}

func testChannelHandler(svc *Services) gomcp.ToolHandlerFor[testChannelInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input testChannelInput) (*gomcp.CallToolResult, any, error) {
		if svc.Channels == nil {
			return errResult("channel store not available")
		}
		if svc.ChannelTester == nil {
			return errResult("notifier not available")
		}
		ch, err := svc.Channels.GetChannel(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("test channel: lookup: %w", err)
		}
		if ch == nil {
			return errResult("channel not found")
		}
		if r, v, err := refuseChannelType(ch.Type); r != nil {
			return r, v, err
		}

		statusCode, testErr := svc.ChannelTester.SendTestWebhook(ctx, ch)
		if testErr != nil {
			return jsonResult(map[string]any{"status": "failed", "error": testErr.Error()})
		}
		return jsonResult(map[string]any{"status": "delivered", "response_code": statusCode})
	}
}
