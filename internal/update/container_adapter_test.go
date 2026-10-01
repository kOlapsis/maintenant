// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeContainerStore struct {
	container.ContainerStore
	list []*container.Container
}

func (f fakeContainerStore) ListContainers(context.Context, container.ListContainersOpts) ([]*container.Container, error) {
	return f.list, nil
}

func (f fakeContainerStore) GetContainerByExternalID(_ context.Context, agentID, externalID string) (*container.Container, error) {
	for _, c := range f.list {
		if c.AgentID == agentID && c.ExternalID == externalID {
			return c, nil
		}
	}
	return nil, nil
}

func (f fakeContainerStore) UpdateContainer(context.Context, *container.Container) error { return nil }

type fakeDetails struct {
	details map[string]RuntimeDetails
	err     error
}

func (f fakeDetails) FetchDetails(context.Context) (map[string]RuntimeDetails, error) {
	return f.details, f.err
}

func newAdapterService(list ...*container.Container) *container.Service {
	return container.NewService(container.Deps{Store: fakeContainerStore{list: list}, Logger: testLogger()})
}

func infosByID(t *testing.T, a *ContainerServiceAdapter) map[string]ContainerInfo {
	t.Helper()
	infos, err := a.ListContainerInfos(context.Background())
	require.NoError(t, err)
	byID := map[string]ContainerInfo{}
	for _, info := range infos {
		byID[info.ExternalID] = info
	}
	return byID
}

func localContainer(id, image string) *container.Container {
	return &container.Container{ExternalID: id, Name: id, Image: image, AgentID: uid.LocalAgent}
}

func agentContainer(agentID, id, image string) *container.Container {
	return &container.Container{
		ExternalID: id, Name: id, Image: image, AgentID: agentID,
		AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
	}
}

func TestContainerServiceAdapter_RepoDigestsAndLocalBuilds(t *testing.T) {
	svc := newAdapterService(
		localContainer("pulled", "nginx:latest"),
		localContainer("built", "myapp:latest"),
		localContainer("unknown", "redis:latest"),
	)
	adapter := NewContainerServiceAdapter(svc, testLogger()).WithDetailsFetcher(fakeDetails{details: map[string]RuntimeDetails{
		"pulled":  {RepoDigests: []string{"nginx@sha256:abc"}, DigestsKnown: true},
		"built":   {DigestsKnown: true},
		"unknown": {},
	}})

	byID := infosByID(t, adapter)
	require.Len(t, byID, 3)
	assert.Equal(t, []string{"nginx@sha256:abc"}, byID["pulled"].RepoDigests)
	assert.False(t, byID["pulled"].LocallyBuilt)
	assert.True(t, byID["built"].LocallyBuilt)
	assert.False(t, byID["unknown"].LocallyBuilt, "a runtime that says nothing about the image must not hide it")
}

func TestContainerServiceAdapter_SwarmTaskNamesItsService(t *testing.T) {
	task := localContainer("task", "nginx:1.26.0")
	task.ApplySwarmTaskLabels(map[string]string{
		"com.docker.swarm.service.id":   "svc-id",
		"com.docker.swarm.service.name": "shop_web",
	})
	svc := newAdapterService(task, localContainer("plain", "redis:7"))

	byID := infosByID(t, NewContainerServiceAdapter(svc, testLogger()))
	assert.Equal(t, "shop_web", byID["task"].SwarmService)
	assert.Empty(t, byID["plain"].SwarmService)
}

func TestContainerServiceAdapter_LocalContainersFollowTheirRuntime(t *testing.T) {
	svc := newAdapterService(localContainer("seen", "nginx:latest"), localContainer("gone", "redis:latest"))
	details := map[string]RuntimeDetails{"seen": {
		Labels:       map[string]string{"maintenant.update.track": "patch"},
		PodContainer: "app",
	}}

	byID := infosByID(t, NewContainerServiceAdapter(svc, testLogger()).WithDetailsFetcher(fakeDetails{details: details}))
	require.Contains(t, byID, "seen")
	assert.Equal(t, "patch", byID["seen"].Labels["maintenant.update.track"])
	assert.Equal(t, "app", byID["seen"].PodContainer)
	assert.NotContains(t, byID, "gone", "a container its runtime no longer lists has no labels to honour")

	failing := NewContainerServiceAdapter(svc, testLogger()).WithDetailsFetcher(fakeDetails{err: errors.New("daemon down")})
	assert.Empty(t, infosByID(t, failing), "without their labels, local containers are left for the next scan")
}

func TestContainerServiceAdapter_AgentContainersUseWhatTheAgentReported(t *testing.T) {
	const agentID = "agent-1"
	svc := newAdapterService(
		agentContainer(agentID, "reported", "nginx:latest"),
		agentContainer(agentID, "silent", "redis:latest"),
	)
	require.NoError(t, svc.HandleAgentEvent(context.Background(), agentID, &agentpb.ContainerEvent{
		ContainerId: "reported",
		Name:        "reported",
		Image:       "nginx:latest",
		Labels: map[string]string{
			"maintenant.update.enabled": "false",
			"com.example.other":         "x",
		},
		RepoDigests: &agentpb.RepoDigests{Digests: []string{"nginx@sha256:running"}},
	}, agentevent.Meta{ObservedAt: time.Now()}))

	adapter := NewContainerServiceAdapter(svc, testLogger()).WithDetailsFetcher(fakeDetails{err: errors.New("local runtime down")})
	byID := infosByID(t, adapter)

	require.Contains(t, byID, "reported")
	assert.Equal(t, agentID, byID["reported"].AgentID)
	assert.Equal(t, map[string]string{"maintenant.update.enabled": "false"}, byID["reported"].Labels)
	assert.Equal(t, []string{"nginx@sha256:running"}, byID["reported"].RepoDigests)
	assert.NotContains(t, byID, "silent", "an agent container is scanned once its agent has reported its labels")

	results, errs := newTestScanner(&stubRegistry{digest: "sha256:new"}, &stubStore{}).
		Scan(context.Background(), []ContainerInfo{byID["reported"]})
	assert.Empty(t, errs)
	assert.Empty(t, results, "maintenant.update.enabled=false applies to an agent container")

	info, err := adapter.GetContainerInfo(context.Background(), "reported")
	require.NoError(t, err)
	assert.Equal(t, "false", info.Labels["maintenant.update.enabled"])
}
