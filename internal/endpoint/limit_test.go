// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/uid"
)

var (
	editionOnce sync.Once
	editionNow  atomic.Value
)

type paidUnlimited struct{ extension.CommunityPolicy }

func (p paidUnlimited) Limit(e extension.Edition, r extension.Resource) int {
	if e != extension.Community {
		return extension.Unlimited
	}
	return p.CommunityPolicy.Limit(e, r)
}

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	editionOnce.Do(func() {
		extension.Register(paidUnlimited{}, nil)
		editionNow.Store(extension.Community)
		extension.CurrentEdition = func() extension.Edition { return editionNow.Load().(extension.Edition) }
	})
	prev := editionNow.Load()
	editionNow.Store(e)
	t.Cleanup(func() { editionNow.Store(prev) })
}

type limitEnv struct {
	t      *testing.T
	ctx    context.Context
	store  *memStore
	engine *CheckEngine
	svc    *Service

	mu      sync.Mutex
	changed map[string][]bool
	removed []string
	found   map[string]bool
}

func newLimitEnv(t *testing.T, store *memStore) *limitEnv {
	t.Helper()
	if store == nil {
		store = newMemStore()
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &limitEnv{t: t, ctx: ctx, store: store, engine: NewCheckEngine(nil, noopLogger()),
		changed: map[string][]bool{}, found: map[string]bool{}}
	e.svc = NewService(Deps{
		Store:  store,
		Engine: e.engine,
		Logger: noopLogger(),
		EventCallback: func(eventType string, data interface{}) {
			d, _ := data.(map[string]interface{})
			id, _ := d["endpoint_id"].(string)
			e.mu.Lock()
			defer e.mu.Unlock()
			switch eventType {
			case event.EndpointLimitChanged:
				e.changed[id] = append(e.changed[id], d["limited"].(bool))
			case event.EndpointDiscovered:
				e.found[id] = d["limited"].(bool)
			}
		},
		EndpointRemovedCallback: func(_ context.Context, id string) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.removed = append(e.removed, id)
		},
	})
	e.svc.Start(ctx)
	t.Cleanup(func() {
		cancel()
		e.engine.Stop()
	})
	return e
}

func ext(name string, gen int) string { return fmt.Sprintf("ext-%s-%d", name, gen) }

func (e *limitEnv) declare(names ...string) {
	for _, n := range names {
		e.svc.SyncEndpoints(e.ctx, n, ext(n, 0), map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})
	}
}

func (e *limitEnv) id(name string) string {
	return uid.EndpointLabel(uid.LocalAgent, name, "maintenant.endpoint.tcp")
}

func (e *limitEnv) limited() []string {
	e.t.Helper()
	eps, err := e.store.ListEndpoints(e.ctx, ListEndpointsOpts{})
	require.NoError(e.t, err)
	out := []string{}
	for _, ep := range eps {
		if ep.Limited {
			out = append(out, ep.ContainerName+ep.Name)
		}
	}
	sort.Strings(out)
	return out
}

func (e *limitEnv) changes() map[string][]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.changed
	e.changed = map[string][]bool{}
	return out
}

func grandfather(t *testing.T, store *memStore) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, ep := range store.endpoints {
		ep.Grandfathered = true
	}
}

func names(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s-%02d", prefix, i))
	}
	return out
}

func TestApplyLimit_UnderTheCapNothingChanges(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 6)...)
	_, err := e.svc.CreateStandalone(e.ctx, "manual", "127.0.0.1:1", TypeTCP, DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, e.svc.ApplyLimit(e.ctx))

	assert.Empty(t, e.limited())
	assert.Empty(t, e.changes())
	assert.Zero(t, e.store.limitWrites, "no limited flag is written under the cap")
	assert.Equal(t, 7, e.engine.ActiveCount())
	for id, limited := range e.found {
		assert.False(t, limited, id)
	}
}

func TestApplyLimit_NewIdentitiesTakeTheFreedSlots(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 10)...)
	for _, n := range names("svc", 1, 6) {
		e.svc.HandleContainerDestroy(e.ctx, ext(n, 0))
	}
	e.declare(names("task", 1, 6)...)

	assert.Empty(t, e.limited(), "a vanished row holds no slot")
	assert.Empty(t, e.changes())
	assert.Zero(t, e.store.limitWrites)
}

func TestApplyLimit_PastTheCapNewestAreLimited(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 12)...)

	assert.Equal(t, []string{"svc-11", "svc-12"}, e.limited())
	assert.Equal(t, 10, e.engine.ActiveCount())
	for _, n := range []string{"svc-11", "svc-12"} {
		ep, err := e.store.GetEndpointByID(e.ctx, e.id(n))
		require.NoError(t, err)
		assert.Equal(t, StatusUnknown, ep.Status)
		assert.Contains(t, e.removed, e.id(n))
		assert.True(t, e.found[e.id(n)], "%s is announced limited", n)
	}
	assert.False(t, e.found[e.id("svc-10")])
	assert.Equal(t, map[string][]bool{e.id("svc-11"): {true}, e.id("svc-12"): {true}}, e.changes())

	writes := e.store.limitWrites
	require.NoError(t, e.svc.ApplyLimit(e.ctx))
	assert.Equal(t, writes, e.store.limitWrites, "a second pass writes nothing")
	assert.Empty(t, e.changes())
}

func TestApplyLimit_FreedSlotGoesToTheOldestWaiting(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	manual, err := e.svc.CreateStandalone(e.ctx, "manual", "127.0.0.1:1", TypeTCP, DefaultConfig())
	require.NoError(t, err)
	e.declare(names("svc", 1, 11)...)
	require.Equal(t, []string{"svc-10", "svc-11"}, e.limited())
	e.changes()

	require.NoError(t, e.svc.Delete(e.ctx, manual.ID))

	assert.Equal(t, []string{"svc-11"}, e.limited())
	assert.Equal(t, map[string][]bool{e.id("svc-10"): {false}}, e.changes())
	assert.Equal(t, 10, e.engine.ActiveCount())
}

func TestApplyLimit_VanishedEndpointFreesItsSlotAtOnce(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 11)...)

	e.svc.HandleContainerDestroy(e.ctx, ext("svc-05", 0))
	assert.Empty(t, e.limited())
	assert.Equal(t, 10, e.engine.ActiveCount())
}

func TestApplyLimit_EditionChanges(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 13)...)
	require.Equal(t, names("svc", 11, 13), e.limited())
	e.changes()

	pinEdition(t, extension.Pro)
	require.NoError(t, e.svc.ApplyLimit(e.ctx))
	assert.Empty(t, e.limited())
	assert.Equal(t, 13, e.engine.ActiveCount())
	assert.Equal(t, map[string][]bool{e.id("svc-11"): {false}, e.id("svc-12"): {false}, e.id("svc-13"): {false}}, e.changes())

	e.store.mu.Lock()
	e.store.endpoints[e.id("svc-12")].Grandfathered = true
	e.store.mu.Unlock()
	pinEdition(t, extension.Community)
	require.NoError(t, e.svc.ApplyLimit(e.ctx))
	assert.Equal(t, []string{"svc-10", "svc-11", "svc-13"}, e.limited(), "grandfathered first, then the oldest")
	assert.Equal(t, 10, e.engine.ActiveCount())
}

func TestApplyLimit_RestartChangesNothing(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 12)...)
	before := e.limited()
	writes := e.store.limitWrites

	again := newLimitEnv(t, e.store)
	require.NoError(t, again.svc.ApplyLimit(again.ctx))
	assert.Equal(t, before, again.limited())
	assert.Equal(t, writes, e.store.limitWrites)
	assert.Empty(t, again.changes())
}

func TestCreateStandalone_RefusedWhenLabelEndpointsFillTheCap(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 9)...)

	_, err := e.svc.CreateStandalone(e.ctx, "fits", "127.0.0.1:1", TypeTCP, DefaultConfig())
	require.NoError(t, err)
	_, err = e.svc.CreateStandalone(e.ctx, "too-many", "127.0.0.1:1", TypeTCP, DefaultConfig())
	require.ErrorIs(t, err, ErrLimitReached)
	assert.Empty(t, e.limited())
}

func TestLimited_EndpointIsNeitherCheckedNorRecorded(t *testing.T) {
	pinEdition(t, extension.Community)
	e := newLimitEnv(t, nil)
	e.declare(names("svc", 1, 11)...)
	id := e.id("svc-11")

	_, err := e.svc.CheckNow(e.ctx, id)
	assert.ErrorIs(t, err, ErrLimited)

	e.svc.ProcessCheckResult(e.ctx, id, CheckResult{EndpointID: id, ErrorMessage: "refused", Timestamp: time.Now()})
	_, total, err := e.store.ListCheckResults(e.ctx, id, ListChecksOpts{})
	require.NoError(t, err)
	assert.Zero(t, total)

	ep, err := e.store.GetEndpointByID(e.ctx, id)
	require.NoError(t, err)
	e.engine.AddEndpoint(e.ctx, ep)
	assert.Equal(t, 10, e.engine.ActiveCount(), "the engine refuses a limited endpoint")
}

func TestGrandfathered_StayMonitoredAcrossRecreationsAndRestarts(t *testing.T) {
	pinEdition(t, extension.Community)
	store := newMemStore()
	old := newLimitEnv(t, store)
	old.declare(names("svc", 1, 13)...)
	old.svc.SyncEndpoints(old.ctx, "batch", ext("batch", 0), map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})
	old.svc.HandleContainerStop(old.ctx, ext("batch", 0))
	pinEdition(t, extension.Pro)
	require.NoError(t, old.svc.ApplyLimit(old.ctx))
	grandfather(t, store)
	pinEdition(t, extension.Community)

	e := newLimitEnv(t, store)
	require.NoError(t, e.svc.ApplyLimit(e.ctx))
	require.Empty(t, e.limited(), "the 14 grandfathered stay monitored")

	e.declare("svc-20-new")
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "a 15th endpoint is limited")

	for gen := 1; gen <= 20; gen++ {
		e.svc.HandleContainerDestroy(e.ctx, ext("svc-03", gen-1))
		e.svc.HandleContainerStart(e.ctx, "svc-03", ext("svc-03", gen), map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})
		require.Equal(t, []string{"svc-20-new"}, e.limited(), "recreation %d", gen)
	}

	e.svc.HandleContainerStart(e.ctx, "batch", ext("batch", 0), map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "a container stopped at the update restarts monitored")
}

func TestGrandfathered_PurgedRowComesBackLimited(t *testing.T) {
	pinEdition(t, extension.Community)
	store := newMemStore()
	e := newLimitEnv(t, store)
	pinEdition(t, extension.Pro)
	e.declare(names("svc", 1, 12)...)
	grandfather(t, store)
	pinEdition(t, extension.Community)

	e.svc.HandleContainerDestroy(e.ctx, ext("svc-05", 0))
	require.NoError(t, store.DeleteEndpoint(e.ctx, e.id("svc-05")))
	e.svc.HandleContainerStart(e.ctx, "svc-05", ext("svc-05", 1), map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})

	assert.Equal(t, []string{"svc-05"}, e.limited(), "past the purge it is a new endpoint")
}

func TestGrandfathered_RemovingLabelsPromotesNobody(t *testing.T) {
	pinEdition(t, extension.Community)
	store := newMemStore()
	e := newLimitEnv(t, store)
	pinEdition(t, extension.Pro)
	e.declare(names("svc", 1, 14)...)
	grandfather(t, store)
	pinEdition(t, extension.Community)
	e.declare("svc-20-new")
	require.Equal(t, []string{"svc-20-new"}, e.limited())

	for _, n := range []string{"svc-02", "svc-07", "svc-12"} {
		e.svc.SyncEndpoints(e.ctx, n, ext(n, 0), map[string]string{})
	}
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "11 grandfathered still exceed the cap")
	assert.Equal(t, 11, e.engine.ActiveCount())
}
