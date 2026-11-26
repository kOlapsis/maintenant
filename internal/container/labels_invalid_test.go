// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/uid"
)

func TestWarnUnknownLabels_ReportsAKnownLabelWithAnInvalidValue(t *testing.T) {
	logger, logs := captureLogs()
	labels := map[string]string{
		"maintenant.ignore":                  "True",
		"maintenant.alert.severity":          "high",
		"maintenant.alert.restart_threshold": "0",
		"maintenant.group":                   "web",
	}

	id := uid.New()
	WarnUnknownLabels(id, labels, logger)
	WarnUnknownLabels(id, labels, logger)

	for key, value := range map[string]string{
		"maintenant.ignore": "True", "maintenant.alert.severity": "high", "maintenant.alert.restart_threshold": "0",
	} {
		lines := warnLines(logs.String(), "label="+key)
		if assert.Len(t, lines, 1, key) {
			assert.Contains(t, lines[0], "value="+value)
		}
	}
	assert.Empty(t, warnLines(logs.String(), "maintenant.group"))

	logger, logs = captureLogs()
	WarnUnknownLabels(uid.New(), map[string]string{
		"maintenant.ignore": "false", "maintenant.alert.severity": "critical", "maintenant.alert.restart_threshold": "5",
	}, logger)
	assert.Empty(t, warnLines(logs.String(), "label="))
}
