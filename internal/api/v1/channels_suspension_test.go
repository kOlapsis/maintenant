// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func seedEveryChannelType(t *testing.T) *store.ChannelStoreImpl {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cs := store.NewChannelStore(storetest.Open(t, logger))
	for _, chType := range []string{"webhook", "discord", "slack", "teams", "telegram", "email"} {
		_, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
			Name: chType, Type: chType, URL: "https://example.com/" + chType, Enabled: true,
		})
		require.NoError(t, err)
	}
	_, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "off", Type: "slack", URL: "https://example.com/off", Enabled: false,
	})
	require.NoError(t, err)
	return cs
}

var suspendedPerEdition = map[extension.Edition]map[string]string{
	extension.Community: {"slack": "pro", "teams": "pro", "telegram": "personal", "email": "personal"},
	extension.Personal:  {"slack": "pro", "teams": "pro"},
	extension.Pro:       {},
}

func TestListChannels_ExposesSuspensionPerEdition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cs := seedEveryChannelType(t)

	for edition, want := range suspendedPerEdition {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)
			h := &AlertHandler{channelStore: cs, broker: NewSSEBroker(logger)}
			rec := httptest.NewRecorder()
			h.HandleListChannels(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil))
			require.Equal(t, http.StatusOK, rec.Code)

			var body struct {
				Channels []struct {
					Name            string `json:"name"`
					Type            string `json:"type"`
					Suspended       bool   `json:"suspended"`
					RequiredEdition string `json:"required_edition"`
				} `json:"channels"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Channels, 7)
			for _, ch := range body.Channels {
				required, suspended := want[ch.Type]
				assert.Equal(t, suspended, ch.Suspended, "%s under %s", ch.Name, edition)
				assert.Equal(t, required, ch.RequiredEdition, "%s under %s", ch.Name, edition)
			}
		})
	}
}

func TestGetEdition_ListsSuspendedChannels(t *testing.T) {
	cs := seedEveryChannelType(t)

	for edition, want := range suspendedPerEdition {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)
			r := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			rec := httptest.NewRecorder()
			r.handleGetEdition(true, HandlerDeps{ChannelStore: cs})(rec, httptest.NewRequest(http.MethodGet, "/api/v1/edition", nil))

			var body struct {
				SuspendedChannels struct {
					Count    int                      `json:"count"`
					Channels []alert.SuspendedChannel `json:"channels"`
				} `json:"suspended_channels"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, len(want), body.SuspendedChannels.Count, "a disabled channel is not counted")
			require.Len(t, body.SuspendedChannels.Channels, len(want))
			for _, ch := range body.SuspendedChannels.Channels {
				assert.Equal(t, want[ch.Type], ch.RequiredEdition)
				assert.NotEqual(t, "off", ch.Name)
			}
		})
	}
}
