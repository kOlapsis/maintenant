// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
)

func TestCreatePolicy_RefusesUnknownChannel(t *testing.T) {
	req := validRequest()
	req.Levels = []esc.LevelReq{{DelaySeconds: 300, ChannelIDs: []string{"1", "unknown"}}}
	_, err := newTestService(newMockStore()).CreatePolicy(context.Background(), req)
	require.ErrorIs(t, err, esc.ErrValidationFailed)
}

func TestCreatePolicy_RefusesRepeatedOrEmptyChannel(t *testing.T) {
	for _, ids := range [][]string{{"1", "1"}, {"1", ""}} {
		req := validRequest()
		req.Levels = []esc.LevelReq{{DelaySeconds: 300, ChannelIDs: ids}}
		_, err := newTestService(newMockStore()).CreatePolicy(context.Background(), req)
		require.ErrorIs(t, err, esc.ErrValidationFailed, "%v", ids)
	}
}

func TestCreatePolicy_RefusesUnknownSeverity(t *testing.T) {
	req := validRequest()
	req.Filters.Severities = []string{"Critical"}
	_, err := newTestService(newMockStore()).CreatePolicy(context.Background(), req)
	require.ErrorIs(t, err, esc.ErrValidationFailed)
}

type failingChannelStore struct{ mockChannelStore }

func (failingChannelStore) GetChannel(context.Context, string) (*alert.NotificationChannel, error) {
	return nil, errors.New("database unavailable")
}

func TestCreatePolicy_ChannelStoreFailureIsNotValidation(t *testing.T) {
	svc := NewService(newMockStore(), &failingChannelStore{}, func() extension.Edition { return extension.Pro },
		&mockSuppressor{}, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	_, err := svc.CreatePolicy(context.Background(), validRequest())
	require.Error(t, err)
	assert.False(t, errors.Is(err, esc.ErrValidationFailed))
}
