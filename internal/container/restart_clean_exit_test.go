// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestService_ProcessEvent_RestartAfterACleanExitTriggersCheck(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("clean"), StateRunning)
	c.ID = "103"
	store.seed(c)

	checker := &mockRestartChecker{result: nil}
	svc := newTestService(store, func(d *Deps) { d.RestartChecker = checker })

	die := makeTestEvent("die", c.ExternalID)
	die.ExitCode = "0"
	svc.ProcessEvent(context.Background(), die)
	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Equal(t, 1, checker.calls, "a container that exits cleanly and is started again restarted")
}
