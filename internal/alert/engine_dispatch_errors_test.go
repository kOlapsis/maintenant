// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func warnLines(logs string) []string {
	var out []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") {
			out = append(out, line)
		}
	}
	return out
}

func TestEngineDispatch_RuleReadFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.fail.set("ListEnabledTriggers", 1)
	f.send(outageEvent("ep-20", alert.SeverityCritical, false))
	f.barrier(t)

	rows := f.activeRows("ep-20")
	require.Len(t, rows, 1)
	assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "the alert must still be notified once")
}

func TestEngineDispatch_ChannelReadFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.fail.set("GetChannel", 1)
	f.send(outageEvent("ep-21", alert.SeverityCritical, false))
	f.barrier(t)

	rows := f.activeRows("ep-21")
	require.Len(t, rows, 1)
	assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "the alert must still be notified once")
}

func TestEngineDispatch_WarnsOnlyWhenAReadFails(t *testing.T) {
	for _, method := range []string{"ListEnabledTriggers", "GetChannel"} {
		t.Run(method+" unreadable", func(t *testing.T) {
			f := newStoreErrorFixture(t)
			f.engine.Start(f.ctx)

			f.fail.set(method, -1)
			f.send(outageEvent("ep-22", alert.SeverityCritical, false))
			f.barrier(t)

			warns := warnLines(f.logs.String())
			require.NotEmpty(t, warns, "an unreadable %s must be logged as a warning", method)
			assert.Contains(t, warns[0], errStoreDown.Error())
		})
	}

	t.Run("no matching rule", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		evt := outageEvent("ep-23", alert.SeverityCritical, false)
		evt.Source = "other"
		f.send(evt)
		f.barrier(t)

		assert.Empty(t, warnLines(f.logs.String()), "an alert that matches no rule is silent by design")
	})
}
