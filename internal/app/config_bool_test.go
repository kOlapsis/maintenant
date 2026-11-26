// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var restrictingBooleans = []string{
	"MAINTENANT_MCP_READ_ONLY",
	"MAINTENANT_DISABLE_TELEMETRY",
	"MAINTENANT_DISABLE_OS_EOL_REFRESH",
}

var booleanVariables = []string{
	"MAINTENANT_SMTP_ALLOW_PLAINTEXT",
	"MAINTENANT_MCP",
	"MAINTENANT_MCP_ALLOW_UNAUTHENTICATED",
	"MAINTENANT_MCP_SOCKET",
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

			cfg := ConfigFromEnv()

			assert.Equal(t, []InvalidSetting{{Name: name, Raw: "ture"}}, cfg.MistypedBooleans)
			assert.NoError(t, cfg.ValidateBooleans())
		})
	}
}

func TestConfigFromEnv_MistypedRestrictingBooleanIsRefused(t *testing.T) {
	for _, name := range restrictingBooleans {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "treu")

			cfg := ConfigFromEnv()

			err := cfg.ValidateBooleans()
			require.ErrorIs(t, err, ErrRestrictingBoolean)
			assert.Contains(t, err.Error(), name)
			assert.Contains(t, err.Error(), "treu")
			assert.Empty(t, cfg.MistypedBooleans)
		})
	}
}

func TestConfigFromEnv_RecognisedBooleansAreNotReported(t *testing.T) {
	for _, raw := range []string{"", "1", "t", "TRUE", "y", "Yes", "on", "0", "f", "False", "n", "no", "OFF"} {
		for _, name := range append(restrictingBooleans, booleanVariables...) {
			t.Setenv(name, raw)
		}

		cfg := ConfigFromEnv()

		assert.Empty(t, cfg.MistypedBooleans, raw)
		assert.NoError(t, cfg.ValidateBooleans(), raw)
	}
}

func TestConfigFromEnv_MistypedBooleanStillReadsAsFalse(t *testing.T) {
	t.Setenv("MAINTENANT_PROXY_LABELS", "ture")

	assert.False(t, ConfigFromEnv().ProxyLabels)
}
