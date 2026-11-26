// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pinByNameStore struct {
	*stubStore
	asked []string
	err   error
}

func (s *pinByNameStore) AdoptVersionPin(_ context.Context, _, _, name, image string) (*VersionPin, error) {
	s.asked = append(s.asked, name+" "+image)
	return nil, s.err
}

func TestScanner_LooksThePinUpByTheNameThatSurvivesARecreation(t *testing.T) {
	store := &pinByNameStore{stubStore: &stubStore{}}
	sc := newTestScanner(&stubRegistry{}, store)

	_, errs := sc.Scan(context.Background(), []ContainerInfo{
		{ExternalID: "ctr1", Name: "app-web-1", Image: "nginx:1.25.3"},
		{ExternalID: "task1", Name: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.25.3", SwarmService: "shop_web"},
	})

	require.Empty(t, errs)
	assert.Equal(t, []string{"app-web-1 nginx:1.25.3", "shop_web nginx:1.25.3"}, store.asked)
}

func TestPinName_KubernetesWorkloadIsNeverFollowedByItsName(t *testing.T) {
	workload := ContainerInfo{ExternalID: "staging/Deployment/api", Name: "api", RuntimeType: "kubernetes"}
	assert.Empty(t, workload.PinName(), "two namespaces can run a workload under the same name")
	assert.Equal(t, "api", ContainerInfo{Name: "api", RuntimeType: "docker"}.PinName())
}

func TestScanner_UnreadablePinIsAScanError(t *testing.T) {
	store := &pinByNameStore{stubStore: &stubStore{}, err: errors.New("database is locked")}
	reg := &stubRegistry{tags: map[string][]string{"library/nginx": {"1.25.3", "1.27.0"}}}
	sc := newTestScanner(reg, store)

	results, errs := sc.Scan(context.Background(), []ContainerInfo{{ExternalID: "ctr1", Name: "web", Image: "nginx:1.25.3"}})

	assert.Empty(t, results, "a container whose pin cannot be read is not reported as updatable")
	require.Len(t, errs, 1)
	assert.Zero(t, reg.listCalls)
}
