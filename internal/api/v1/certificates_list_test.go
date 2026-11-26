// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestCertificateList_LatestCheckCarriesDaysRemaining(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	certs := store.NewCertificateStore(storetest.Open(t, logger))
	now := time.Now()
	monitor := func(host string) string {
		id, err := certs.CreateMonitor(ctx, &certificate.CertMonitor{
			Hostname: host, Port: 443, Source: certificate.SourceStandalone, Status: certificate.StatusUnknown,
			CheckIntervalSeconds: 43200, CreatedAt: now,
		})
		require.NoError(t, err)
		return id
	}
	notAfter := now.Add(10*24*time.Hour + time.Hour)
	_, err := certs.InsertCheckResult(ctx, &certificate.CertCheckResult{MonitorID: monitor("ok.example.com"), NotAfter: &notAfter, CheckedAt: now})
	require.NoError(t, err)
	_, err = certs.InsertCheckResult(ctx, &certificate.CertCheckResult{MonitorID: monitor("down.example.com"), ErrorMessage: "dial timeout", CheckedAt: now})
	require.NoError(t, err)
	monitor("new.example.com")

	rec := httptest.NewRecorder()
	NewCertificateHandler(certificate.NewService(certificate.Deps{Store: certs, Logger: logger})).
		HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Certificates []struct {
			Hostname    string         `json:"hostname"`
			LatestCheck map[string]any `json:"latest_check"`
		} `json:"certificates"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Certificates, 3)
	for _, c := range body.Certificates {
		switch c.Hostname {
		case "ok.example.com":
			assert.EqualValues(t, 10, c.LatestCheck["days_remaining"])
			assert.NotEmpty(t, c.LatestCheck["not_after"], "the other fields stay")
		case "down.example.com":
			require.NotNil(t, c.LatestCheck)
			assert.NotContains(t, c.LatestCheck, "days_remaining")
		default:
			assert.Nil(t, c.LatestCheck)
		}
	}
}
