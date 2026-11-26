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
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	commesc "github.com/kolapsis/maintenant/internal/commercial/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestDeleteChannel_RefusedWhileAnEscalationPolicyUsesIt(t *testing.T) {
	withEdition(t, extension.Pro)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	chID, err := channels.InsertChannel(ctx, &alert.NotificationChannel{
		Name: "ops-webhook", Type: "webhook", URL: "https://example.com/hook", Enabled: true,
	})
	require.NoError(t, err)
	svc := commesc.NewService(store.NewEscalationStore(db), channels,
		func() extension.Edition { return extension.Pro }, noopSuppressor{}, logger)
	policy, err := svc.CreatePolicy(ctx, escalation.PolicyRequest{
		Name:    "night shift",
		Filters: escalation.Filters{Severities: []string{"critical"}, Scopes: []escalation.Scope{}},
		Levels:  []escalation.LevelReq{{DelaySeconds: 300, ChannelIDs: []string{chID}}},
	})
	require.NoError(t, err)

	h := NewRouter(HandlerDeps{
		Logger:        logger,
		Broker:        NewSSEBroker(logger),
		AlertStore:    store.NewAlertStore(db),
		ChannelStore:  channels,
		EscalationSvc: svc,
	}).Handler()
	del := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+chID, nil))
		return rec
	}

	rec := del()
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "CHANNEL_IN_USE", body.Error.Code)
	assert.Contains(t, body.Error.Message, "night shift")
	kept, err := channels.GetChannel(ctx, chID)
	require.NoError(t, err)
	require.NotNil(t, kept)

	t.Run("an edition that cannot edit policies still deletes", func(t *testing.T) {
		withEdition(t, extension.Community)
		names, err := escalation.PoliciesUsingChannel(ctx, svc, chID)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	require.NoError(t, svc.DeletePolicy(ctx, policy.ID))
	assert.Equal(t, http.StatusNoContent, del().Code)
}
