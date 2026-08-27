// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/resource"
)

// This file is the matrix of contracts/resource-history.md and
// contracts/resource-top.md: for every edition and every window, both history
// endpoints must answer the same way. A window the product does not know is a
// bad request; one it knows but the edition does not open is an edition
// refusal; and the two are never confused.

// stubHistoryService serves one point for any window, so the test measures the
// gate and not the data.
type stubHistoryService struct {
	calls []string
}

func (s *stubHistoryService) GetHistory(_ context.Context, _ string, window string) ([]*resource.ResourceSnapshot, resource.Granularity, error) {
	s.calls = append(s.calls, window)
	return []*resource.ResourceSnapshot{{ContainerID: "c1", CPUPercent: 12, Timestamp: time.Now()}}, resource.Granularity1h, nil
}

func historyRequest(t *testing.T, svc *stubHistoryService, window string) *httptest.ResponseRecorder {
	t.Helper()
	h := &ResourceHandler{history: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/resources/history?range="+window, nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleGetHistory(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	detail, ok := body["error"].(map[string]any)
	require.True(t, ok, "refusal has no error object: %s", rec.Body.String())

	// FR-012: a refusal never carries data. Truncating to the cap silently, or
	// answering with a partial series, is exactly what the specification bans.
	assert.NotContains(t, body, "points", "a refusal must not carry points")
	return detail
}

// windowMatrix is the table both endpoints are held to. An empty required
// edition means the window is open.
var windowMatrix = map[extension.Edition]map[string]string{
	extension.Community: {
		"1h": "", "6h": "",
		"24h": "personal", "7d": "personal", "30d": "personal",
		"90d": "pro",
	},
	extension.Personal: {
		"1h": "", "6h": "", "24h": "", "7d": "", "30d": "",
		"90d": "pro",
	},
	extension.Pro: {
		"1h": "", "6h": "", "24h": "", "7d": "", "30d": "", "90d": "",
	},
}

func TestHistoryWindows_Matrix(t *testing.T) {
	for edition, windows := range windowMatrix {
		for window, required := range windows {
			t.Run(string(edition)+"/"+window, func(t *testing.T) {
				withEdition(t, edition)
				svc := &stubHistoryService{}
				rec := historyRequest(t, svc, window)

				if required == "" {
					require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
					assert.Equal(t, []string{window}, svc.calls, "the window must reach the service verbatim")
					return
				}

				require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
				assert.Empty(t, svc.calls, "a refused window must never reach the service")

				detail := decodeError(t, rec)
				assert.Equal(t, "EDITION_REQUIRED", detail["code"])
				assert.Equal(t, required, detail["required_edition"])
				assert.Equal(t, window, detail["window"])
				assert.Equal(t, string(extension.CapResourceHistory), detail["feature"])

				// The cap is the running edition's own, not the required one's:
				// it is what the interface shows as "you have up to this".
				expectedCap := map[extension.Edition]string{
					extension.Community: "6h", extension.Personal: "30d", extension.Pro: "90d",
				}[edition]
				assert.Equal(t, expectedCap, detail["max_window"])
			})
		}
	}
}

// stubTopService records which periods reach the store layer.
type stubTopService struct {
	mockResourceTopService
	calls []string
}

func (s *stubTopService) GetTopConsumersByPeriod(_ context.Context, _ string, period string, _ int, _ *string) ([]resource.TopConsumerRow, error) {
	s.calls = append(s.calls, period)
	return []resource.TopConsumerRow{{ContainerID: "c1", AvgValue: 10, AvgPercent: 10}}, nil
}

func topRequest(t *testing.T, svc *stubTopService, query string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewResourceTopHandler(svc)
	rec := httptest.NewRecorder()
	h.HandleGetTopConsumers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/resources/top?metric=cpu&"+query, nil))
	return rec
}

// The same matrix on the top consumers. Community x 30d is the bypass this
// feature closes: it answered 200 to any edition before (FR-014).
func TestTopConsumerWindows_Matrix(t *testing.T) {
	for edition, windows := range windowMatrix {
		for window, required := range windows {
			t.Run(string(edition)+"/"+window, func(t *testing.T) {
				withEdition(t, edition)
				svc := &stubTopService{}
				rec := topRequest(t, svc, "period="+window)

				if required == "" {
					require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
					assert.Equal(t, []string{window}, svc.calls)
					return
				}

				require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
				assert.Empty(t, svc.calls, "a refused window must never reach the store")

				detail := decodeError(t, rec)
				assert.Equal(t, "EDITION_REQUIRED", detail["code"])
				assert.Equal(t, required, detail["required_edition"])
				assert.Equal(t, window, detail["window"])
			})
		}
	}
}

// The host filter is not a way around the cap.
func TestTopConsumerWindows_HostFilterDoesNotOpenTheCap(t *testing.T) {
	withEdition(t, extension.Community)
	for _, host := range []string{"", "local", "agent-9"} {
		svc := &stubTopService{}
		rec := topRequest(t, svc, "period=30d&agent_id="+host)
		assert.Equal(t, http.StatusForbidden, rec.Code, "agent_id=%q", host)
		assert.Empty(t, svc.calls)
	}
}

// An unknown period is a bad request now, where it used to fall through to the
// realtime ranking without saying anything (FR-011).
func TestTopConsumerWindows_UnknownPeriodIsABadRequest(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &stubTopService{}
	rec := topRequest(t, svc, "period=2d")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "INVALID_PERIOD", decodeError(t, rec)["code"])
	assert.Empty(t, svc.calls)
}

// No period at all is the realtime ranking, open everywhere (FR-024).
func TestTopConsumerWindows_RealtimeStaysOpen(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		withEdition(t, edition)
		svc := &stubTopService{}
		rec := topRequest(t, svc, "")

		assert.Equal(t, http.StatusOK, rec.Code, "edition %q", edition)
		assert.Empty(t, svc.calls, "the realtime ranking does not read history")
	}
}

// A window the product does not know is a bad request, distinct from an edition
// refusal by both code and status (FR-011).
func TestHistoryWindows_UnknownWindowIsABadRequest(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		for _, window := range []string{"12h", "2d", "365d", "1H"} {
			t.Run(string(edition)+"/"+window, func(t *testing.T) {
				withEdition(t, edition)
				svc := &stubHistoryService{}
				rec := historyRequest(t, svc, window)

				require.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Empty(t, svc.calls)
				assert.Equal(t, "INVALID_RANGE", decodeError(t, rec)["code"])
			})
		}
	}
}

// An absent range keeps its historical default rather than becoming an error.
func TestHistoryWindows_AbsentRangeDefaultsToOneHour(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &stubHistoryService{}
	h := &ResourceHandler{history: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/resources/history", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleGetHistory(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"1h"}, svc.calls)
}
