// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRegistry struct {
	stubRegistry
	queried []string
}

func (r *recordingRegistry) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	r.queried = append(r.queried, imageRef)
	return r.stubRegistry.ListTags(ctx, imageRef)
}

func (r *recordingRegistry) ResolveDigests(ctx context.Context, imageRef string) (RemoteDigests, error) {
	r.queried = append(r.queried, imageRef)
	return r.stubRegistry.ResolveDigests(ctx, imageRef)
}

func TestScanner_RegistryLabelQueriesTheMirror(t *testing.T) {
	cases := []struct {
		image, repo string
	}{
		{"postgres:16.1", "mirror.example.com:5000/library/postgres"},
		{"acme/api:16.1", "mirror.example.com:5000/acme/api"},
		{"ghcr.io/acme/api:16.1", "mirror.example.com:5000/acme/api"},
	}
	for _, tc := range cases {
		t.Run(tc.image, func(t *testing.T) {
			reg := &recordingRegistry{stubRegistry: stubRegistry{
				tags:   map[string][]string{tc.repo: {"16.1", "16.2"}},
				digest: "sha256:new",
			}}
			sc := newTestScanner(reg, &stubStore{})

			results, errs := sc.Scan(context.Background(), []ContainerInfo{{
				ExternalID: "ctr1", Name: "db", Image: tc.image,
				Labels: map[string]string{"maintenant.update.registry": "mirror.example.com:5000"},
			}})
			require.Empty(t, errs)
			require.Len(t, results, 1)
			assert.Equal(t, "16.2", results[0].LatestTag)
			assert.Equal(t, "mirror.example.com:5000", results[0].Registry)
			assert.Equal(t, []string{tc.repo, tc.repo + ":16.2"}, reg.queried)
		})
	}
}
