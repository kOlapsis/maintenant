// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestHandleAgentEvent_RefusesNewMonitorsOnceTheAgentHoldsTheCeiling(t *testing.T) {
	const agentA, agentB = "00000000-0000-0000-0000-0000000000c1", "00000000-0000-0000-0000-0000000000c2"
	pinEdition(t, extension.Pro)
	certificate.SetMaxAgentLabelMonitors(t, 2)

	ctx := context.Background()
	db := storetest.Open(t, nil)
	for _, id := range []string{agentA, agentB} {
		require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
			AgentID: id, Hostname: id, Label: id, OSArch: "linux/amd64",
			AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
		}))
	}
	st := store.NewCertificateStore(db)
	logs := &bytes.Buffer{}
	svc := certificate.NewService(certificate.Deps{Store: st, Logger: slog.New(slog.NewTextHandler(logs, nil))})

	scan := func(agentID, host string) error {
		return svc.HandleAgentEvent(ctx, agentID, &agentpb.CertificateInfo{
			Host: host, Port: 443, SubjectCn: host,
			NotBefore: timestamppb.New(time.Now().Add(-time.Hour)),
			NotAfter:  timestamppb.New(time.Now().Add(90 * 24 * time.Hour)),
		})
	}
	count := func(agentID string) int {
		monitors, err := st.ListMonitors(ctx, certificate.ListCertificatesOpts{AgentFilter: &agentID, IncludeInactive: true})
		require.NoError(t, err)
		return len(monitors)
	}

	require.NoError(t, scan(agentA, "a.example.com"))
	require.NoError(t, scan(agentA, "b.example.com"))

	for _, host := range []string{"c.example.com", "d.example.com", "e.example.com"} {
		require.ErrorIs(t, scan(agentA, host), agentevent.ErrInvalid, "a refused scan must not be replayed by the agent")
	}
	assert.Equal(t, 2, count(agentA), "the agent's monitors stop at the ceiling")
	assert.Equal(t, 1, strings.Count(logs.String(), "certificate monitor ceiling"), "the refusal is logged once per agent")

	require.NoError(t, scan(agentA, "a.example.com"), "monitors the server already holds keep being updated")
	require.NoError(t, scan(agentB, "c.example.com"), "another agent is not affected")
	assert.Equal(t, 1, count(agentB))
}
