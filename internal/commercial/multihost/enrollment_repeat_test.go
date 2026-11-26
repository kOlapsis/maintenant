// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestRegisterAgent_ARepeatedEnrollmentIsNeitherLoggedNorAnnounced(t *testing.T) {
	orig := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = orig })

	ctx := context.Background()
	logs := &bytes.Buffer{}
	sink := &recordingBroadcaster{}
	agents := store.NewAgentStore(storetest.Open(t, nil))
	impl := &ingestImpl{deps: Deps{AgentStore: agents, Broadcaster: sink, Logger: slog.New(slog.NewTextHandler(logs, nil))}}

	const agentID = "55555555-5555-5555-5555-555555555555"
	tok := insertToken(t, agents, "tok-repeat", "mnt_enr_repeattoken00001")
	req := registerReq(agentID, tok, "nas")
	created := func() int {
		n := 0
		for _, b := range sink.sent {
			if b.eventType == event.AgentCreated {
				n++
			}
		}
		return n
	}
	registered := func() int { return strings.Count(logs.String(), "agent.registered") }

	_, err := impl.RegisterAgent(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 1, created())
	require.Equal(t, 1, registered())

	_, err = impl.RegisterAgent(ctx, req)
	require.NoError(t, err, "the agent whose answer was lost is answered again")
	assert.Equal(t, 1, created(), "a repeat creates no agent")
	assert.Equal(t, 1, registered(), "a repeat is not a registration")

	require.NoError(t, agents.Revoke(ctx, agentID, "admin"))
	_, err = impl.RegisterAgent(ctx, req)
	assert.Equal(t, codes.FailedPrecondition, grpcstatus.Code(err), "a revoked agent cannot replay its token: %v", err)
	assert.Equal(t, 1, created())
	assert.Equal(t, 1, registered())
}
