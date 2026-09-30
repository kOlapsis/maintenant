// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/update"
)

// updateDetailsFetcher returns the source of the update scan's live container details for the local runtime, nil when it has none.
func updateDetailsFetcher(rt runtime.Runtime) update.DetailsFetcher {
	switch r := rt.(type) {
	case *docker.Runtime:
		return &dockerDetailsFetcher{rt: r}
	case *kubernetes.Runtime:
		return &kubernetesDetailsFetcher{rt: r}
	}
	return nil
}

// dockerDetailsFetcher reads the labels and image digests of the Docker containers at scan time, without persisting them.
type dockerDetailsFetcher struct {
	rt *docker.Runtime
}

func (f *dockerDetailsFetcher) FetchDetails(ctx context.Context) (map[string]update.RuntimeDetails, error) {
	results, err := f.rt.DiscoverAllWithLabels(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch container labels: %w", err)
	}
	digests, err := f.rt.ContainerRepoDigests(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch image digests: %w", err)
	}
	out := make(map[string]update.RuntimeDetails, len(results))
	for _, r := range results {
		id := r.Container.ExternalID
		d, known := digests[id]
		out[id] = update.RuntimeDetails{Labels: r.Labels, RepoDigests: d, DigestsKnown: known}
	}
	return out, nil
}

// kubernetesDetailsFetcher reads the update annotations, pod container and running image digests of the workloads at scan time.
type kubernetesDetailsFetcher struct {
	rt *kubernetes.Runtime
}

func (f *kubernetesDetailsFetcher) FetchDetails(ctx context.Context) (map[string]update.RuntimeDetails, error) {
	images, err := f.rt.WorkloadImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch workload images: %w", err)
	}
	out := make(map[string]update.RuntimeDetails, len(images))
	for id, w := range images {
		out[id] = update.RuntimeDetails{
			Labels:       w.Annotations,
			RepoDigests:  w.RepoDigests,
			DigestsKnown: w.DigestsKnown,
			PodContainer: w.Container,
		}
	}
	return out, nil
}
