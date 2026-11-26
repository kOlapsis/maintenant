// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cleanExitService serves one running local container, discovered with or without a restart policy that restarts it after a clean exit.
func cleanExitService(t *testing.T, restartsOnCleanExit bool) (*Service, *svcStore, *mockRestartChecker, string) {
	t.Helper()
	store := newSvcStore()
	c := makeTestContainer(extID("clean"), StateRunning)
	store.seed(c)
	checker := &mockRestartChecker{}
	svc := newTestService(store, func(d *Deps) { d.RestartChecker = checker })
	discovered := *c
	discovered.RestartsOnCleanExit = restartsOnCleanExit
	require.NoError(t, svc.Reconcile(context.Background(), &mockDiscoverer{containers: []*Container{&discovered}}))
	return svc, store, checker, c.ExternalID
}

func cleanDie(externalID, code string) ContainerEvent {
	die := makeTestEvent("die", externalID)
	die.ExitCode = code
	return die
}

func TestService_ProcessEvent_RestartPolicyRestartingACleanExitCounts(t *testing.T) {
	ctx := context.Background()
	svc, store, checker, id := cleanExitService(t, true)

	svc.ProcessEvent(ctx, cleanDie(id, "0"))
	assert.Equal(t, StateRestarting, store.storedState(id), "the runtime is about to start it again")
	svc.ProcessEvent(ctx, makeTestEvent("start", id))

	assert.Equal(t, 1, checker.calls)
}

func TestService_ProcessEvent_StartAfterACleanExitWithoutRestartPolicyDoesNotCount(t *testing.T) {
	ctx := context.Background()
	svc, store, checker, id := cleanExitService(t, false)

	svc.ProcessEvent(ctx, cleanDie(id, "0"))
	assert.Equal(t, StateCompleted, store.storedState(id))
	svc.ProcessEvent(ctx, makeTestEvent("start", id))

	assert.Zero(t, checker.calls, "a scheduler or a person started it again")
}

func TestService_ProcessEvent_DockerRestartOfAnAlwaysContainerDoesNotCount(t *testing.T) {
	ctx := context.Background()
	svc, store, checker, id := cleanExitService(t, true)

	svc.ProcessEvent(ctx, cleanDie(id, "143"))
	svc.ProcessEvent(ctx, makeTestEvent("stop", id))
	assert.Equal(t, StateCompleted, store.storedState(id), "a stop requested by hand ends the restart policy's work")
	svc.ProcessEvent(ctx, makeTestEvent("start", id))

	assert.Zero(t, checker.calls)
}
