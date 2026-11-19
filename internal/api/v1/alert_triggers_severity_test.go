// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriggers_RefuseAnUnknownSeverityFilter(t *testing.T) {
	h := NewAlertTriggerHandler(nil, nil, nil)

	for _, bad := range []string{"Critical", "critical,urgent", "warn"} {
		rec := call(h.HandleCreateTrigger, http.MethodPost, `{"name":"x","filter_severities":"`+bad+`","channel_ids":["c1"]}`)

		assert.Equal(t, http.StatusBadRequest, rec.Code, bad)
		assert.True(t, strings.Contains(rec.Body.String(), "filter_severities"), rec.Body.String())
	}
}

func TestValidateTriggerInput_AcceptsTheKnownSeverities(t *testing.T) {
	for _, good := range []string{"", "critical", "critical, warning", "info,warning,critical", "critical,"} {
		in := triggerInput{Name: "x", FilterSeverities: good, ChannelIDs: []string{"c1"}}
		require.NoError(t, validateTriggerInput(&in), good)
	}
}
