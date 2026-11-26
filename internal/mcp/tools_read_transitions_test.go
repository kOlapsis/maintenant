// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestGetContainer_ReturnsOnlyTheLatestTransitions(t *testing.T) {
	const seeded = getContainerTransitions + 10
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	cs := store.NewContainerStore(db)
	ctx := context.Background()
	now := time.Now()
	cid, err := cs.InsertContainer(ctx, &container.Container{
		ExternalID: "ext-restart-loop", Name: "api", Image: "img:v1", State: container.StateRunning,
		AlertSeverity: container.SeverityWarning, RestartThreshold: 3, RuntimeType: "docker",
		FirstSeenAt: now, LastStateChangeAt: now,
	})
	require.NoError(t, err)
	start := now.Add(-time.Hour)
	for i := range seeded {
		_, err := cs.InsertTransition(ctx, &container.StateTransition{
			ContainerID: cid, PreviousState: container.StateExited, NewState: container.StateRunning,
			Timestamp: start.Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
	}
	svc := &Services{Containers: container.NewService(container.Deps{Store: cs, Logger: logger}), Logger: logger, Version: "test"}

	result, _, err := getContainerHandler(svc)(ctx, nil, getContainerInput{ContainerID: cid})
	require.NoError(t, err)
	require.False(t, result.IsError)

	var got struct {
		Transitions []struct {
			Timestamp time.Time `json:"timestamp"`
		} `json:"transitions"`
	}
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &got))
	require.Len(t, got.Transitions, getContainerTransitions)
	assert.Equal(t, start.Add((seeded-1)*time.Second).Unix(), got.Transitions[0].Timestamp.Unix(), "newest first")
}
