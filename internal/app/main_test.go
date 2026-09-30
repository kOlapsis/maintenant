// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"testing"

	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	// Tests that call App.Start must never report to the real telemetry endpoint.
	if err := os.Setenv("DO_NOT_TRACK", "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
