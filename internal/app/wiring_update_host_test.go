// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log"
	"log/slog"
	"maps"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

// FR-004, US2 scenarios 1 and 2, contract table "Texte des alertes": a remote
// host is named in both the detection and the recovery text.
func TestUpdateAlertText_NamesTheRemoteHost(t *testing.T) {
	cases := []struct {
		container    string
		agentID      string
		host         string
		latest       string
		wantDetected string
		wantResolved string
	}{
		{"valkey", "agent-nas", "NAS cave", "8.1.3",
			"Update available for valkey on NAS cave: 8.1.3",
			"Update no longer required for valkey on NAS cave"},
		{"immich_redis", "agent-web", "web-07", "7.4.1",
			"Update available for immich_redis on web-07: 7.4.1",
			"Update no longer required for immich_redis on web-07"},
	}
	for _, tc := range cases {
		t.Run(tc.container, func(t *testing.T) {
			m := detectedPayload(tc.container, "ctr-"+tc.container, 50)
			m["latest_tag"] = tc.latest
			m["agent_id"] = tc.agentID
			m["agent_name"] = tc.host

			detected := updateDetectedAlert(m, false)
			assert.Equal(t, tc.wantDetected, detected.Message)
			assert.Equal(t, tc.container, detected.EntityName, "the entity stays the container")
			assert.Equal(t, tc.agentID, detected.AgentID)

			resolved := updateResolvedAlert(m)
			assert.Equal(t, tc.wantResolved, resolved.Message)
			assert.Equal(t, tc.container, resolved.EntityName)
			assert.Equal(t, tc.agentID, resolved.AgentID)
		})
	}
}

// FR-004, FR-007, US2 scenario 4: the local runtime keeps today's text, whether
// agent_name is absent (the contract) or left empty.
func TestUpdateAlertText_LocalRuntimeUnchanged(t *testing.T) {
	for label, agentName := range map[string]*string{"absent": nil, "empty": new(string)} {
		t.Run(label, func(t *testing.T) {
			m := detectedPayload("valkey", "ctr-valkey", 50)
			m["latest_tag"] = "8.1.3"
			m["agent_id"] = uid.LocalAgent
			if agentName != nil {
				m["agent_name"] = *agentName
			}
			assert.Equal(t, "Update available for valkey: 8.1.3", updateDetectedAlert(m, false).Message)
			assert.Equal(t, "Update no longer required for valkey", updateResolvedAlert(m).Message)
		})
	}
}

func callAgentName(t *testing.T, a *App, agentID string) (got string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("agentName(%q) panicked: %v", agentID, r)
		}
	}()
	return a.agentName(context.Background(), agentID)
}

// FR-004 and contract: the host is the agent's label, else its hostname, else its id.
func TestAgentName_LabelElseHostnameElseID(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	agentStore := store.NewAgentStore(db)
	for _, ag := range []agent.Agent{
		{AgentID: "agent-nas", Hostname: "nas-02", Label: "NAS cave", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
		{AgentID: "agent-web", Hostname: "web-07", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
	} {
		require.NoError(t, agentStore.Insert(ctx, &ag))
	}
	a := &App{agentStore: agentStore, logger: logger}

	assert.Equal(t, "NAS cave", callAgentName(t, a, "agent-nas"))
	assert.Equal(t, "web-07", callAgentName(t, a, "agent-web"))
	assert.Equal(t, "agent-gone", callAgentName(t, a, "agent-gone"))
}

type appScanLister struct {
	mu    sync.Mutex
	infos []update.ContainerInfo
}

func (l *appScanLister) ListContainerInfos(context.Context) ([]update.ContainerInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.infos), nil
}

func (l *appScanLister) setImage(externalID, image string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.infos {
		if l.infos[i].ExternalID == externalID {
			l.infos[i].Image = image
		}
	}
}

func (l *appScanLister) add(info update.ContainerInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, info)
}

func (l *appScanLister) remove(externalID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = slices.DeleteFunc(l.infos, func(c update.ContainerInfo) bool { return c.ExternalID == externalID })
}

func startAppUpdateRegistry(t *testing.T, tags map[string][]string) string {
	t.Helper()
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	for repo, list := range tags {
		for _, tag := range list {
			img, err := random.Image(64, 1)
			require.NoError(t, err)
			ref, err := name.ParseReference(host + "/" + repo + ":" + tag)
			require.NoError(t, err)
			require.NoError(t, remote.Write(ref, img))
		}
	}
	return host
}

type hostedImage struct {
	agentID    string
	externalID string
}

// updateAlertsFixture runs real scans against an in-memory registry, with the
// real update store, the real alert engine and the real update callback. Only
// the containers each runtime reports are simulated.
type updateAlertsFixture struct {
	ctx        context.Context
	app        *App
	alerts     *store.AlertStoreImpl
	agents     *store.AgentStore
	containers *store.ContainerStore
	updates    *store.UpdateStore
	lister     *appScanLister
	observed   chan v1.SSEEvent
	registry   string
	name       string
	repo       string
}

func newUpdateAlertsFixture(t *testing.T, agents []agent.Agent, containerName, repo string, tags []string, hosts []hostedImage) *updateAlertsFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	alertStore := store.NewAlertStore(db)
	agentStore := store.NewAgentStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})
	engine.Start(ctx)

	for i := range agents {
		require.NoError(t, agentStore.Insert(ctx, &agents[i]))
	}

	f := &updateAlertsFixture{
		ctx:        ctx,
		alerts:     alertStore,
		agents:     agentStore,
		containers: store.NewContainerStore(db),
		updates:    store.NewUpdateStore(db),
		lister:     &appScanLister{},
		observed:   make(chan v1.SSEEvent, 256),
		registry:   startAppUpdateRegistry(t, map[string][]string{repo: tags}),
		name:       containerName,
		repo:       repo,
	}
	for _, h := range hosts {
		f.run(t, h, tags[0])
	}

	broker := v1.NewSSEBroker(logger)
	broker.AddObserver(f.observed)
	f.app = &App{
		logger:       logger,
		alertStore:   alertStore,
		agentStore:   agentStore,
		alertEngine:  engine,
		statusSvc:    status.NewService(status.Deps{Components: store.NewStatusComponentStore(db), Logger: logger}),
		broker:       broker,
		containerSvc: container.NewService(container.Deps{Store: f.containers, Logger: logger}),
		updateSvc: update.NewService(update.Deps{
			Store:      f.updates,
			Scanner:    update.NewScanner(update.NewRegistryClient(), f.updates, logger),
			Containers: f.lister,
			Logger:     logger,
		}),
	}
	f.app.wireUpdateCallback()
	return f
}

func (f *updateAlertsFixture) image(tag string) string {
	return f.registry + "/" + f.repo + ":" + tag
}

// run records a running container on its host and makes the runtime report it.
func (f *updateAlertsFixture) run(t *testing.T, h hostedImage, tag string) {
	t.Helper()
	now := time.Now()
	id, err := f.containers.InsertContainer(f.ctx, &container.Container{
		ExternalID:        h.externalID,
		AgentID:           h.agentID,
		Name:              f.name,
		Image:             f.image(tag),
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	})
	require.NoError(t, err)
	f.lister.add(update.ContainerInfo{
		UID: id, AgentID: h.agentID, ExternalID: h.externalID, Name: f.name, Image: f.image(tag), RuntimeType: "docker",
	})
}

// recreate plays `docker compose pull && docker compose up -d` on one host: the
// old container is destroyed (archived) and a new one runs under a new id.
func (f *updateAlertsFixture) recreate(t *testing.T, old hostedImage, newExternalID, tag string) {
	t.Helper()
	require.NoError(t, f.containers.ArchiveContainer(f.ctx, uid.Container(old.agentID, old.externalID), time.Now()))
	f.lister.remove(old.externalID)
	f.run(t, hostedImage{old.agentID, newExternalID}, tag)
}

// scan runs one scan and returns the events it broadcast, by type then container id.
func (f *updateAlertsFixture) scan(t *testing.T) map[string]map[string]map[string]any {
	t.Helper()
	idle := func() bool { return !f.app.updateSvc.IsScanning() }
	require.Eventually(t, idle, 10*time.Second, 10*time.Millisecond)
	_, err := f.app.updateSvc.TriggerScan(f.ctx)
	require.NoError(t, err)
	events := map[string]map[string]map[string]any{}
	timeout := time.After(60 * time.Second)
	for {
		select {
		case evt := <-f.observed:
			if evt.Type == event.UpdateScanCompleted {
				require.Eventually(t, idle, 10*time.Second, 10*time.Millisecond)
				return events
			}
			m, ok := evt.Data.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["container_id"].(string)
			if events[evt.Type] == nil {
				events[evt.Type] = map[string]map[string]any{}
			}
			events[evt.Type][id] = maps.Clone(m)
		case <-timeout:
			t.Fatal("the scan never completed")
		}
	}
}

// activeUpdateAlerts returns the active update alerts, by agent id.
func (f *updateAlertsFixture) activeUpdateAlerts(t *testing.T) map[string]*alert.Alert {
	t.Helper()
	active, err := f.alerts.ListActiveAlerts(f.ctx)
	assert.NoError(t, err)
	out := map[string]*alert.Alert{}
	for _, al := range active {
		if al.Source == "update" && al.AlertType == "update_available" {
			assert.Nil(t, out[al.AgentID], "two active update alerts for host %s", al.AgentID)
			out[al.AgentID] = al
		}
	}
	return out
}

func (f *updateAlertsFixture) waitActive(t *testing.T, agentIDs ...string) map[string]*alert.Alert {
	t.Helper()
	var active map[string]*alert.Alert
	require.Eventually(t, func() bool {
		active = f.activeUpdateAlerts(t)
		return len(active) == len(agentIDs)
	}, 5*time.Second, 10*time.Millisecond, "expected active update alerts on %v", agentIDs)
	assert.ElementsMatch(t, agentIDs, slices.Collect(maps.Keys(active)))
	return active
}

func (f *updateAlertsFixture) recoveries(t *testing.T) []string {
	t.Helper()
	all, err := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: "update", Status: alert.StatusResolved, Limit: 100})
	assert.NoError(t, err)
	var out []string
	for _, al := range all {
		if strings.HasPrefix(al.Message, "Update no longer required") {
			out = append(out, al.Message)
		}
	}
	return out
}

func (f *updateAlertsFixture) waitRecoveries(t *testing.T, want ...string) {
	t.Helper()
	var got []string
	require.Eventually(t, func() bool {
		got = f.recoveries(t)
		return len(got) >= len(want)
	}, 5*time.Second, 10*time.Millisecond, "the recoveries are recorded")
	assert.ElementsMatch(t, want, got)
}

// entries returns the update entries the Updates page lists, container id to host.
func (f *updateAlertsFixture) entries(t *testing.T) map[string]string {
	t.Helper()
	listed, err := f.updates.ListImageUpdates(f.ctx, update.ListImageUpdatesOpts{})
	require.NoError(t, err)
	out := map[string]string{}
	for _, u := range listed {
		out[u.ContainerID] = u.AgentID
	}
	return out
}

// US2 scenarios 1 to 4, FR-003, FR-004, SC-002, SC-003, edge case "Agent renommé"
// and the agent_name contract of update.detected / update.resolved.
func TestUpdateAlerts_OnePerHostThroughTheEngine(t *testing.T) {
	local := hostedImage{uid.LocalAgent, "ctr-redis-local"}
	rackB := hostedImage{"agent-rack-b", "ctr-redis-rackb"}
	edge := hostedImage{"agent-edge", "ctr-redis-edge"}
	f := newUpdateAlertsFixture(t,
		[]agent.Agent{
			{AgentID: "agent-rack-b", Hostname: "rack-b.lan", Label: "Rack B", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
			{AgentID: "agent-edge", Hostname: "edge-03", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
		},
		"redis-cache", "cache/redis", []string{"7.2.4", "7.4.1"},
		[]hostedImage{local, rackB, edge})

	events := f.scan(t)

	detected := events[event.UpdateDetected]
	require.Len(t, detected, 3)
	assert.Equal(t, "Rack B", detected["ctr-redis-rackb"]["agent_name"])
	assert.Equal(t, "edge-03", detected["ctr-redis-edge"]["agent_name"])
	assert.NotContains(t, detected["ctr-redis-local"], "agent_name", "the local runtime has no agent_name")

	active := f.waitActive(t, uid.LocalAgent, "agent-rack-b", "agent-edge")
	assert.Equal(t, "Update available for redis-cache on Rack B: 7.4.1", active["agent-rack-b"].Message)
	assert.Equal(t, "Update available for redis-cache on edge-03: 7.4.1", active["agent-edge"].Message)
	assert.Equal(t, "Update available for redis-cache: 7.4.1", active[uid.LocalAgent].Message)
	for _, al := range active {
		assert.Equal(t, "redis-cache", al.EntityName)
	}

	require.NoError(t, f.agents.UpdateLabel(f.ctx, "agent-edge", "Edge Z"))
	f.lister.setImage(rackB.externalID, f.image("7.4.1"))
	events = f.scan(t)

	resolved := events[event.UpdateResolved]
	require.Len(t, resolved, 1, "only the upgraded host's update is resolved")
	require.Contains(t, resolved, "ctr-redis-rackb")
	assert.Equal(t, "Rack B", resolved["ctr-redis-rackb"]["agent_name"])
	assert.Equal(t, "agent-rack-b", resolved["ctr-redis-rackb"]["agent_id"])

	active = f.waitActive(t, uid.LocalAgent, "agent-edge")
	assert.Equal(t, "Update available for redis-cache on edge-03: 7.4.1", active["agent-edge"].Message,
		"an alert already sent keeps the host name it carried")

	f.waitRecoveries(t, "Update no longer required for redis-cache on Rack B")
}

// US2 scenario 5, US1 scenario 6, FR-009, contract "Résolution": recreating the
// container under a new id resolves the alert of that host only, and drops its
// entry. Played on a remote host, then on the local runtime.
func TestUpdateAlerts_RecreatedContainerResolvesItsHostAlert(t *testing.T) {
	local := hostedImage{uid.LocalAgent, "ctr-valkey-local"}
	rackB := hostedImage{"agent-rack-b", "ctr-valkey-rackb"}
	edge := hostedImage{"agent-edge", "ctr-valkey-edge"}
	f := newUpdateAlertsFixture(t,
		[]agent.Agent{
			{AgentID: "agent-rack-b", Hostname: "rack-b.lan", Label: "Rack B", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
			{AgentID: "agent-edge", Hostname: "edge-03", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
		},
		"valkey", "valkey/valkey", []string{"8.0.1", "8.1.3"},
		[]hostedImage{local, rackB, edge})

	f.scan(t)
	active := f.waitActive(t, uid.LocalAgent, "agent-rack-b", "agent-edge")
	rackBUID := uid.Container(rackB.agentID, rackB.externalID)
	require.Equal(t, rackBUID, active["agent-rack-b"].EntityID, "precondition: the alert is keyed on the container uid")

	f.recreate(t, rackB, "ctr-valkey-rackb-2", "8.1.3")
	events := f.scan(t)

	resolved := events[event.UpdateResolved]
	require.Contains(t, resolved, rackB.externalID)
	assert.Equal(t, rackBUID, resolved[rackB.externalID]["container_uid"], "the recovery names the old container's uid")
	assert.Equal(t, "agent-rack-b", resolved[rackB.externalID]["agent_id"])
	assert.Equal(t, "Rack B", resolved[rackB.externalID]["agent_name"])

	active = f.waitActive(t, uid.LocalAgent, "agent-edge")
	f.waitRecoveries(t, "Update no longer required for valkey on Rack B")
	assert.Equal(t, map[string]string{
		local.externalID: uid.LocalAgent,
		edge.externalID:  "agent-edge",
	}, f.entries(t))

	f.recreate(t, local, "ctr-valkey-local-2", "8.1.3")
	f.scan(t)

	edgeAlert := active["agent-edge"]
	active = f.waitActive(t, "agent-edge")
	assert.Equal(t, edgeAlert.ID, active["agent-edge"].ID, "the other host's alert is left alone")
	f.waitRecoveries(t, "Update no longer required for valkey on Rack B", "Update no longer required for valkey")
	assert.Equal(t, map[string]string{edge.externalID: "agent-edge"}, f.entries(t))
}

// US1 scenario 5, FR-003, edge case "Hôte absent du scan", contract "Résolution":
// a scan that misses a host (its agent is not back yet after a server restart)
// leaves that host's entry and alert as they are, even though the same container
// is up to date on another host. Once every host is scanned again, the other
// host's update is cleaned and the absent host's alert was never re-raised.
func TestUpdateAlerts_AbsentHostKeepsItsAlert(t *testing.T) {
	nas := hostedImage{"agent-nas", "ctr-immich-redis-nas"}
	web := hostedImage{"agent-web", "ctr-immich-redis-web"}
	f := newUpdateAlertsFixture(t,
		[]agent.Agent{
			{AgentID: "agent-nas", Hostname: "nas-02", Label: "NAS cave", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
			{AgentID: "agent-web", Hostname: "web-07", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
		},
		"immich_redis", "cache/redis", []string{"7.2.4", "7.4.1"},
		[]hostedImage{nas, web})

	f.scan(t)
	nasAlert := f.waitActive(t, "agent-nas", "agent-web")["agent-nas"]

	f.lister.remove(nas.externalID)
	f.lister.setImage(web.externalID, f.image("7.4.1"))
	events := f.scan(t)

	assert.NotContains(t, events[event.UpdateResolved], nas.externalID, "the absent host's update is not resolved")
	assert.Contains(t, f.entries(t), nas.externalID, "the absent host keeps its entry")
	if al := f.activeUpdateAlerts(t)["agent-nas"]; assert.NotNil(t, al, "the absent host keeps its alert") {
		assert.Equal(t, nasAlert.ID, al.ID)
	}

	f.run(t, nas, "7.2.4")
	f.scan(t)

	active := f.waitActive(t, "agent-nas")
	assert.Equal(t, nasAlert.ID, active["agent-nas"].ID, "the absent host's alert was never resolved nor raised again")
	assert.Equal(t, map[string]string{nas.externalID: "agent-nas"}, f.entries(t))
	f.waitRecoveries(t, "Update no longer required for immich_redis on web-07")
}
