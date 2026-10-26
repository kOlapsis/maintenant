// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: lowering a timeout to the interval is routine, not a warning.

// FR-012, US4 scenario 4: "endpoint timeout > interval, adjusting" is logged at INFO.
func TestParseEndpointLabels_TimeoutAdjustmentIsInformational(t *testing.T) {
	for _, tc := range []struct{ interval, timeout string }{{"10s", "30s"}, {"5s", "1m"}} {
		t.Run(tc.interval+"_"+tc.timeout, func(t *testing.T) {
			var logs strings.Builder
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

			parsed, errs := ParseEndpointLabels(map[string]string{
				"maintenant.endpoint.http":     "http://web:8080/health",
				"maintenant.endpoint.interval": tc.interval,
				"maintenant.endpoint.timeout":  tc.timeout,
			}, logger)
			require.Empty(t, errs)
			require.Len(t, parsed, 1)
			assert.Equal(t, parsed[0].Config.Interval, parsed[0].Config.Timeout, "the timeout is still lowered to the interval")

			var line string
			for _, l := range strings.Split(logs.String(), "\n") {
				if strings.Contains(l, "endpoint timeout > interval, adjusting") {
					line = l
				}
			}
			require.NotEmpty(t, line, "the adjustment is still logged:\n%s", logs.String())
			assert.Contains(t, line, "level=INFO")
			assert.NotContains(t, logs.String(), "level=WARN")
		})
	}
}
