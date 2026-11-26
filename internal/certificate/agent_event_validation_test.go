// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestHandleAgentEvent_AppliesTheValidationTheAgentReports(t *testing.T) {
	const agentID = "00000000-0000-0000-0000-000000000061"

	for name, tc := range map[string]struct {
		chainValid, hostnameMatch *bool
		chainError                string
		wantChainValid, wantMatch bool
		wantAlerts                []string
	}{
		"chain invalid": {
			chainValid: proto.Bool(false), chainError: "x509: certificate signed by unknown authority", hostnameMatch: proto.Bool(true),
			wantChainValid: false, wantMatch: true, wantAlerts: []string{certificate.AlertTypeChainInvalid},
		},
		"hostname mismatch": {
			chainValid: proto.Bool(true), hostnameMatch: proto.Bool(false),
			wantChainValid: true, wantMatch: false, wantAlerts: []string{certificate.AlertTypeHostnameMismatch},
		},
		"valid": {
			chainValid: proto.Bool(true), hostnameMatch: proto.Bool(true),
			wantChainValid: true, wantMatch: true,
		},
		"validation absent": {
			wantChainValid: true, wantMatch: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			pinEdition(t, extension.Community)
			ctx := context.Background()
			db := storetest.Open(t, nil)
			st := store.NewCertificateStore(db)
			require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
				AgentID: agentID, Hostname: "nas", Label: "nas", OSArch: "linux/amd64",
				AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
			}))

			var mu sync.Mutex
			var alerts []string
			svc := certificate.NewService(certificate.Deps{
				Store:  st,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				EventCallback: func(eventType string, data interface{}) {
					if eventType != event.CertificateAlert {
						return
					}
					m, _ := data.(map[string]interface{})
					alertType, _ := m["alert_type"].(string)
					mu.Lock()
					defer mu.Unlock()
					alerts = append(alerts, alertType)
				},
			})

			require.NoError(t, svc.HandleAgentEvent(ctx, agentID, &agentpb.CertificateInfo{
				Host: "nas.example.com", Port: 443, SubjectCn: "nas.example.com",
				NotBefore:  timestamppb.New(time.Now().Add(-time.Hour)),
				NotAfter:   timestamppb.New(time.Now().Add(90 * 24 * time.Hour)),
				ChainValid: tc.chainValid, ChainError: tc.chainError, HostnameMatch: tc.hostnameMatch,
			}))

			monitors, err := st.ListMonitors(ctx, certificate.ListCertificatesOpts{})
			require.NoError(t, err)
			require.Len(t, monitors, 1)
			assert.Equal(t, certificate.StatusValid, monitors[0].Status)

			latest, err := st.GetLatestCheckResult(ctx, monitors[0].ID)
			require.NoError(t, err)
			require.NotNil(t, latest)
			require.NotNil(t, latest.ChainValid)
			require.NotNil(t, latest.HostnameMatch)
			assert.Equal(t, tc.wantChainValid, *latest.ChainValid)
			assert.Equal(t, tc.chainError, latest.ChainError)
			assert.Equal(t, tc.wantMatch, *latest.HostnameMatch)

			mu.Lock()
			defer mu.Unlock()
			assert.ElementsMatch(t, tc.wantAlerts, alerts)
		})
	}
}
