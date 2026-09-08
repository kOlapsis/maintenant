// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package endpoint

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

func probedEngine(t *testing.T) (*CheckEngine, chan string) {
	t.Helper()
	checks := make(chan string, 64)
	engine := NewCheckEngine(func(endpointID string, _ CheckResult) {
		select {
		case checks <- endpointID:
		default:
		}
	}, noopLogger())
	return engine, checks
}

func listenerTarget(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return lis.Addr().String()
}

func awaitCheck(t *testing.T, checks chan string, endpointID string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case id := <-checks:
			if id == endpointID {
				return
			}
		case <-deadline:
			t.Fatalf("endpoint %s was never probed", endpointID)
		}
	}
}

func TestService_Start_ReprobesStandaloneAfterRestart(t *testing.T) {
	store := newMemStore()
	target := listenerTarget(t)

	firstEngine, firstChecks := probedEngine(t)
	first := NewService(Deps{Store: store, Engine: firstEngine, Logger: noopLogger()})
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	require.NoError(t, first.Start(firstCtx))

	ep, err := first.CreateStandalone(context.Background(), "std", target, TypeTCP, DefaultConfig())
	require.NoError(t, err)
	awaitCheck(t, firstChecks, ep.ID)

	first.Stop()
	cancelFirst()

	secondEngine, secondChecks := probedEngine(t)
	second := NewService(Deps{Store: store, Engine: secondEngine, Logger: noopLogger()})
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	t.Cleanup(func() { cancelSecond(); second.Stop() })
	require.NoError(t, second.Start(secondCtx))

	awaitCheck(t, secondChecks, ep.ID)
	assert.Equal(t, 1, secondEngine.ActiveCount())
}

func TestService_ReloadActiveEndpoints_Idempotent(t *testing.T) {
	store := newMemStore()
	target := listenerTarget(t)

	engine, checks := probedEngine(t)
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); svc.Stop() })
	require.NoError(t, svc.Start(ctx))

	ep, err := svc.CreateStandalone(context.Background(), "std", target, TypeTCP, DefaultConfig())
	require.NoError(t, err)
	awaitCheck(t, checks, ep.ID)

	require.NoError(t, svc.ReloadActiveEndpoints(ctx))
	require.NoError(t, svc.ReloadActiveEndpoints(ctx))

	assert.Equal(t, 1, engine.ActiveCount())
}

func TestService_ReloadActiveEndpoints_SkipsAgentOwned(t *testing.T) {
	store := newMemStore()
	target := listenerTarget(t)

	remoteID := seedEndpoint(t, store, &Endpoint{
		Name: "remote", Target: target, EndpointType: TypeTCP,
		Source: SourceLabel, AgentID: uid.New(),
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})
	localID := seedEndpoint(t, store, &Endpoint{
		Name: "local", Target: target, EndpointType: TypeTCP,
		Source: SourceLabel, AgentID: uid.LocalAgent, LabelKey: "local",
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})

	engine, checks := probedEngine(t)
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); svc.Stop() })
	require.NoError(t, svc.Start(ctx))

	awaitCheck(t, checks, localID)
	assert.Equal(t, 1, engine.ActiveCount())

	select {
	case id := <-checks:
		assert.NotEqual(t, remoteID, id, "the server probed an endpoint owned by a remote agent")
	case <-time.After(200 * time.Millisecond):
	}
}
