// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var booleanVariables = []string{
	"MAINTENANT_SMTP_ALLOW_PLAINTEXT",
	"MAINTENANT_MCP",
	"MAINTENANT_MCP_ALLOW_UNAUTHENTICATED",
	"MAINTENANT_MCP_READ_ONLY",
	"MAINTENANT_MCP_SOCKET",
	"MAINTENANT_DISABLE_TELEMETRY",
	"MAINTENANT_DISABLE_OS_EOL_REFRESH",
	"MAINTENANT_PROXY_LABELS",
	"MAINTENANT_ALLOW_PRIVATE_WEBHOOKS",
	"MAINTENANT_GRPC_TLS_INSECURE",
	"MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY",
	"MAINTENANT_EMBEDDED_AGENT",
}

func TestConfigFromEnv_MistypedBooleanIsReported(t *testing.T) {
	for _, name := range booleanVariables {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, " ture ")

			assert.Equal(t, []InvalidSetting{{Name: name, Raw: "ture"}}, ConfigFromEnv().MistypedBooleans)
		})
	}
}

func TestConfigFromEnv_RecognisedBooleansAreNotReported(t *testing.T) {
	for _, raw := range []string{"", "1", "t", "TRUE", "y", "Yes", "on", "0", "f", "False", "n", "no", "OFF"} {
		for _, name := range booleanVariables {
			t.Setenv(name, raw)
		}

		assert.Empty(t, ConfigFromEnv().MistypedBooleans, raw)
	}
}

func TestConfigFromEnv_MistypedBooleanStillReadsAsFalse(t *testing.T) {
	t.Setenv("MAINTENANT_DISABLE_TELEMETRY", "ture")

	assert.False(t, ConfigFromEnv().DisableTelemetry)
}
