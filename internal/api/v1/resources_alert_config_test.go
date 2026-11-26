// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type unusedRuntime struct{ runtime.Runtime }

type failingConfigRead struct {
	resource.ResourceStore
	fail bool
}

func (f *failingConfigRead) GetAlertConfig(ctx context.Context, id string) (*resource.ResourceAlertConfig, error) {
	if f.fail {
		return nil, errors.New("read failed")
	}
	return f.ResourceStore.GetAlertConfig(ctx, id)
}

type resourceAlertBench struct {
	h           *ResourceHandler
	svc         *resource.Service
	rs          *store.ResourceStore
	reads       *failingConfigRead
	containerID string

	mu     sync.Mutex
	events []string
}

func newResourceAlertBench(t *testing.T) *resourceAlertBench {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	cs := store.NewContainerStore(db)
	now := time.Now()
	id, err := cs.InsertContainer(context.Background(), &container.Container{
		ExternalID: "ext-res-alert", Name: "web", Image: "img:v1",
		State: container.StateRunning, AlertSeverity: container.SeverityWarning,
		RestartThreshold: 3, RuntimeType: "docker", FirstSeenAt: now, LastStateChangeAt: now,
	})
	require.NoError(t, err)

	b := &resourceAlertBench{rs: store.NewResourceStore(db), containerID: id}
	b.reads = &failingConfigRead{ResourceStore: b.rs}
	containers := container.NewService(container.Deps{Store: cs, Logger: logger})
	b.svc = resource.NewService(resource.Deps{
		Store:        b.reads,
		Runtime:      unusedRuntime{},
		ContainerSvc: containers,
		Logger:       logger,
		EventCallback: func(eventType string, data interface{}) {
			m, _ := data.(map[string]interface{})
			if eventType == event.ResourceAlert || eventType == event.ResourceRecovery {
				b.mu.Lock()
				defer b.mu.Unlock()
				alertType, _ := m["alert_type"].(string)
				recovered, _ := m["recovered_type"].(string)
				b.events = append(b.events, eventType+":"+alertType+recovered)
			}
		},
	})
	b.h = NewResourceHandler(b.svc, containers)
	return b
}

func (b *resourceAlertBench) put(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/containers/"+b.containerID+"/resources/alerts", strings.NewReader(body))
	req.SetPathValue("id", b.containerID)
	rec := httptest.NewRecorder()
	b.h.HandleUpsertAlertConfig(rec, req)
	return rec
}

func (b *resourceAlertBench) sample(t *testing.T, cpu float64) {
	t.Helper()
	require.NoError(t, b.svc.HandleAgentEvent(context.Background(), "", &agentpb.ResourceSample{
		ContainerId: "ext-res-alert", CpuPercent: cpu,
	}, agentevent.Meta{ObservedAt: time.Now()}))
}

func (b *resourceAlertBench) stored(t *testing.T) *resource.ResourceAlertConfig {
	t.Helper()
	cfg, err := b.rs.GetAlertConfig(context.Background(), b.containerID)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

func (b *resourceAlertBench) takeEvents() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.events
	b.events = nil
	return out
}

func (b *resourceAlertBench) raiseCPUAlert(t *testing.T) {
	t.Helper()
	require.Equal(t, http.StatusOK, b.put(t, `{"cpu_threshold":50,"mem_threshold":90,"enabled":true}`).Code)
	b.sample(t, 99)
	b.sample(t, 99)
	require.Equal(t, resource.AlertStateCPU, b.stored(t).AlertState)
	require.Equal(t, []string{"resource.alert:cpu"}, b.takeEvents())
}

func TestResourceAlertConfig_DisablingAnAlertingConfigRecoversIt(t *testing.T) {
	b := newResourceAlertBench(t)
	b.raiseCPUAlert(t)

	require.Equal(t, http.StatusOK, b.put(t, `{"cpu_threshold":50,"mem_threshold":90,"enabled":false}`).Code)

	assert.Equal(t, []string{"resource.recovery:cpu"}, b.takeEvents(), "the open alert is recovered")
	cfg := b.stored(t)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, resource.AlertStateNormal, cfg.AlertState)
	assert.Zero(t, cfg.CPUConsecutiveBreaches)
	assert.Zero(t, cfg.MemConsecutiveBreaches)

	require.Equal(t, http.StatusOK, b.put(t, `{"cpu_threshold":50,"mem_threshold":90,"enabled":true}`).Code)
	assert.Empty(t, b.takeEvents(), "re-enabling alone raises nothing")
	b.sample(t, 99)
	b.sample(t, 99)
	assert.Equal(t, []string{"resource.alert:cpu"}, b.takeEvents(), "a new breach alerts again")
	assert.Equal(t, resource.AlertStateCPU, b.stored(t).AlertState)
}

func TestResourceAlertConfig_PutWithoutEnabledKeepsTheConfigActive(t *testing.T) {
	b := newResourceAlertBench(t)
	b.raiseCPUAlert(t)

	rec := b.put(t, `{"cpu_threshold":60,"mem_threshold":80}`)
	require.Equal(t, http.StatusOK, rec.Code)

	cfg := b.stored(t)
	assert.True(t, cfg.Enabled, "an absent enabled keeps the stored value")
	assert.Equal(t, 60.0, cfg.CPUThreshold)
	assert.Equal(t, 80.0, cfg.MemThreshold)
	assert.Equal(t, resource.AlertStateCPU, cfg.AlertState, "the open alert is untouched")
	assert.Empty(t, b.takeEvents())
}

func TestResourceAlertConfig_ReadErrorFailsTheRequestAndWritesNothing(t *testing.T) {
	b := newResourceAlertBench(t)
	b.raiseCPUAlert(t)
	before := b.stored(t)

	b.reads.fail = true
	rec := b.put(t, `{"cpu_threshold":10,"mem_threshold":10,"enabled":false}`)
	b.reads.fail = false

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	after := b.stored(t)
	assert.Equal(t, before.CPUThreshold, after.CPUThreshold)
	assert.Equal(t, before.MemThreshold, after.MemThreshold)
	assert.True(t, after.Enabled)
	assert.Equal(t, resource.AlertStateCPU, after.AlertState)
	assert.Empty(t, b.takeEvents())
}

func TestResourceAlertConfig_UnknownContainerIsNotFound(t *testing.T) {
	b := newResourceAlertBench(t)

	put := httptest.NewRequest(http.MethodPut, "/api/v1/containers/gone/resources/alerts", strings.NewReader(`{"cpu_threshold":50,"mem_threshold":90,"enabled":true}`))
	put.SetPathValue("id", "gone")
	rec := httptest.NewRecorder()
	b.h.HandleUpsertAlertConfig(rec, put)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	get := httptest.NewRequest(http.MethodGet, "/api/v1/containers/gone/resources/alerts", nil)
	get.SetPathValue("id", "gone")
	rec = httptest.NewRecorder()
	b.h.HandleGetAlertConfig(rec, get)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
