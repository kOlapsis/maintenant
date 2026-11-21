// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/uid"
)

func imageSupportService(t *testing.T, containers []ContainerInfo, withTable bool) *Service {
	t.Helper()
	deps := Deps{Store: &stubStore{}, Scanner: newTestScanner(&stubRegistry{}, &stubStore{}), Containers: stubLister{containers: containers}, Logger: testLogger()}
	if withTable {
		deps.EOL = postgresLifecycle(t)
	}
	return NewService(deps)
}

func TestListImageSupport_ListsTrackedImagesWhateverTheUpdateTracking(t *testing.T) {
	svc := imageSupportService(t, []ContainerInfo{
		{ExternalID: "c-ended", Name: "db-old", Image: "postgres:14.12"},
		{ExternalID: "c-soon", Name: "db-soon", Image: "postgres:15.7", Labels: map[string]string{"maintenant.update.enabled": "false"}},
		{ExternalID: "c-pinned", Name: "db-pinned", Image: "library/postgres:16.3", Labels: map[string]string{"maintenant.update.track": "patch"}},
		{ExternalID: "c-other", Name: "cache", Image: "acme/postgres:14.12"},
		{ExternalID: "c-unknown", Name: "db-ancient", Image: "postgres:9.1.2"},
		{ExternalID: "c-built", Name: "db-built", Image: "postgres:14.12", LocallyBuilt: true},
	}, true)

	got, err := svc.ListImageSupport(context.Background())
	require.NoError(t, err)

	require.Len(t, got, 3)
	byName := map[string]ImageSupport{}
	for _, g := range got {
		byName[g.ContainerName] = g
	}
	assert.Equal(t, eol.StateEnded, byName["db-old"].Support.State)
	assert.Equal(t, "c-ended", byName["db-old"].ContainerID)
	assert.Equal(t, "postgres:14.12", byName["db-old"].Image)
	assert.Equal(t, "postgresql", byName["db-old"].Support.Product)
	assert.Equal(t, "14", byName["db-old"].Support.Cycle)
	assert.Equal(t, eol.StateEndingSoon, byName["db-soon"].Support.State, "tracking disabled does not hide the image")
	assert.Equal(t, eol.StateSupported, byName["db-pinned"].Support.State, "a restricted track does not hide the image")
	assert.NotContains(t, byName, "cache", "not an official image")
	assert.NotContains(t, byName, "db-ancient", "unknown release line")
	assert.NotContains(t, byName, "db-built", "locally built image")
}

func TestListImageSupport_WithoutTableProviderIsEmpty(t *testing.T) {
	svc := imageSupportService(t, []ContainerInfo{{ExternalID: "c1", Name: "db", Image: "postgres:14.12"}}, false)

	got, err := svc.ListImageSupport(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestListImageSupport_OrdersByNameThenAgent(t *testing.T) {
	svc := imageSupportService(t, []ContainerInfo{
		{ExternalID: "c3", Name: "db", AgentID: "agent-b", Image: "postgres:14.12"},
		{ExternalID: "c1", Name: "web", AgentID: "agent-a", Image: "postgres:14.12"},
		{ExternalID: "c2", Name: "db", AgentID: "agent-a", Image: "postgres:14.12"},
		{ExternalID: "c4", Name: "db", Image: "postgres:14.12"},
	}, true)

	got, err := svc.ListImageSupport(context.Background())
	require.NoError(t, err)

	var order []string
	for _, g := range got {
		order = append(order, g.ContainerName+"/"+g.AgentID)
	}
	assert.Equal(t, []string{
		"db/" + uid.LocalAgent,
		"db/agent-a",
		"db/agent-b",
		"web/agent-a",
	}, order)
}
