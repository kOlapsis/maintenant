// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/extension"
)

func asEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func TestHighAvailabilityOptionsInUse(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want []string
	}{
		{"none", Config{}, nil},
		{"state dir is free", Config{StateDir: "/var/lib/maintenant"}, nil},
		{"synchronous normal", Config{SQLiteSynchronous: "NORMAL"}, nil},
		{"synchronous full", Config{SQLiteSynchronous: "FULL"}, []string{"MAINTENANT_SQLITE_SYNCHRONOUS=FULL"}},
		{"synchronous full lowercase", Config{SQLiteSynchronous: " full "}, []string{"MAINTENANT_SQLITE_SYNCHRONOUS=FULL"}},
		{"synchronous invalid", Config{SQLiteSynchronous: "EXTRA"}, nil},
		{"require state dir", Config{RequireStateDir: true}, []string{"MAINTENANT_REQUIRE_STATE_DIR"}},
		{"require existing data", Config{RequireExistingData: true}, []string{"MAINTENANT_REQUIRE_EXISTING_DATA"}},
		{"all", Config{
			StateDir: "/srv", SQLiteSynchronous: "full", RequireStateDir: true, RequireExistingData: true,
		}, []string{
			"MAINTENANT_SQLITE_SYNCHRONOUS=FULL",
			"MAINTENANT_REQUIRE_STATE_DIR", "MAINTENANT_REQUIRE_EXISTING_DATA",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, highAvailabilityOptionsInUse(tc.cfg))
		})
	}
}

func allHighAvailabilityOptions() Config {
	return Config{StateDir: "/srv/maintenant", SQLiteSynchronous: "FULL", RequireStateDir: true, RequireExistingData: true}
}

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestApplyHighAvailabilityPolicy_IgnoredBelowPro(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal} {
		t.Run(string(edition), func(t *testing.T) {
			asEdition(t, edition)
			logger, logs := captureLogger()

			got, ignored := applyHighAvailabilityPolicy(allHighAvailabilityOptions(), logger)

			assert.Equal(t, []string{
				"MAINTENANT_SQLITE_SYNCHRONOUS=FULL",
				"MAINTENANT_REQUIRE_STATE_DIR", "MAINTENANT_REQUIRE_EXISTING_DATA",
			}, ignored)
			assert.Equal(t, "NORMAL", got.SQLiteSynchronous)
			assert.False(t, got.RequireStateDir)
			assert.False(t, got.RequireExistingData)
			assert.Equal(t, "/srv/maintenant", got.StateDir)

			out := logs.String()
			assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("level=WARN")))
			for _, name := range ignored {
				assert.Contains(t, out, name)
			}
			assert.Contains(t, out, "required_edition="+string(extension.MinEdition(extension.CapHighAvailability)))
			assert.Contains(t, out, "current_edition="+string(edition))
		})
	}
}

func TestApplyHighAvailabilityPolicy_KeptWithPro(t *testing.T) {
	asEdition(t, extension.Pro)
	logger, logs := captureLogger()

	got, ignored := applyHighAvailabilityPolicy(allHighAvailabilityOptions(), logger)

	assert.Nil(t, ignored)
	assert.Equal(t, allHighAvailabilityOptions(), got)
	assert.Empty(t, logs.String())
}

func TestApplyHighAvailabilityPolicy_StateDirIsFree(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			asEdition(t, edition)
			logger, logs := captureLogger()
			cfg := Config{StateDir: "/srv/maintenant"}

			got, ignored := applyHighAvailabilityPolicy(cfg, logger)

			assert.Nil(t, ignored)
			assert.Equal(t, cfg, got)
			assert.Empty(t, logs.String())
		})
	}
}
