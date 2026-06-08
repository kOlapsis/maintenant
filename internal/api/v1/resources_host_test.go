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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/resource"
)

func ptr(s string) *string { return &s }

func TestParseHostFilter(t *testing.T) {
	cases := []struct {
		query string
		want  *string
	}{
		{"", nil},
		{"agent_id=", nil},
		{"agent_id=local", ptr("")},
		{"agent_id=agent-9", ptr("agent-9")},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil)
		got := parseHostFilter(r)
		if tc.want == nil {
			assert.Nil(t, got, tc.query)
		} else {
			require.NotNil(t, got, tc.query)
			assert.Equal(t, *tc.want, *got, tc.query)
		}
	}
}

func TestHostMatches(t *testing.T) {
	local := ""
	agent := "a1"
	other := "a2"

	// nil filter => all hosts match.
	assert.True(t, hostMatches(nil, nil))
	assert.True(t, hostMatches(&agent, nil))

	// local filter ("") => only NULL-agent (local) snapshots.
	assert.True(t, hostMatches(nil, &local))
	assert.False(t, hostMatches(&agent, &local))

	// specific agent filter.
	assert.True(t, hostMatches(&agent, &agent))
	assert.False(t, hostMatches(&other, &agent))
	assert.False(t, hostMatches(nil, &agent))
}

// The realtime top-consumers path must honour the ?agent_id host filter.
func TestHandleGetTopConsumers_RealtimeHostFilter(t *testing.T) {
	agentID := "a1"
	svc := &mockResourceTopService{
		snapshots: map[int64]*resource.ResourceSnapshot{
			1: {ContainerID: 1, CPUPercent: 10, AgentID: nil},      // local
			2: {ContainerID: 2, CPUPercent: 20, AgentID: &agentID}, // agent
		},
		names: map[int64]string{1: "local-ctr", 2: "agent-ctr"},
	}
	h := NewResourceTopHandler(svc)

	get := func(query string) []map[string]any {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/resources/top?metric=cpu&"+query, nil)
		w := httptest.NewRecorder()
		h.HandleGetTopConsumers(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Consumers []map[string]any `json:"consumers"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Consumers
	}

	all := get("")
	assert.Len(t, all, 2)

	local := get("agent_id=local")
	require.Len(t, local, 1)
	assert.Equal(t, float64(1), local[0]["container_id"])

	agent := get("agent_id=a1")
	require.Len(t, agent, 1)
	assert.Equal(t, float64(2), agent[0]["container_id"])
}
