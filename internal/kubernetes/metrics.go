// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// podMetricsCacheTTL bounds the age of the shared pod-metrics snapshot.
// Must be shorter than the resource collector interval so each cycle triggers
// one LIST, but long enough that concurrent lookups inside a single cycle
// share that LIST instead of issuing per-pod GETs.
const podMetricsCacheTTL = 5 * time.Second

// PodResourceMetrics holds aggregated resource metrics for a single pod.
type PodResourceMetrics struct {
	Name          string
	Namespace     string
	CPUMillicores int64
	MemBytes      int64
	MemLimitBytes int64
	Timestamp     time.Time
}

// NodeResourceMetrics holds resource metrics for a single node.
type NodeResourceMetrics struct {
	Name                  string
	CPUMillicores         int64
	CPUCapacityMillicores int64
	MemBytes              int64
	MemCapacityBytes      int64
	Timestamp             time.Time
}

// cachedPodMetrics returns a pod's metrics from the shared cache, refreshing
// via a single cluster-wide LIST when the cache is stale. This keeps request
// rate on metrics.k8s.io at O(1) per collection cycle instead of O(pods),
// avoiding client-go's default 5 QPS / burst 10 throttle on large clusters.
func (r *Runtime) cachedPodMetrics(ctx context.Context, namespace, name string) (*metricsv1beta1.PodMetrics, error) {
	mc := r.metricsClient()
	if mc == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	r.podMetricsMu.Lock()
	defer r.podMetricsMu.Unlock()

	if time.Since(r.podMetricsAt) > podMetricsCacheTTL || r.podMetricsCache == nil {
		list, err := mc.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list pod metrics: %w", err)
		}
		fresh := make(map[string]*metricsv1beta1.PodMetrics, len(list.Items))
		for i := range list.Items {
			pm := &list.Items[i]
			fresh[pm.Namespace+"/"+pm.Name] = pm
		}
		r.podMetricsCache = fresh
		r.podMetricsAt = time.Now()
	}

	pm, ok := r.podMetricsCache[namespace+"/"+name]
	if !ok {
		return nil, fmt.Errorf("pod metrics not found: %s/%s", namespace, name)
	}
	return pm, nil
}

// GetPodMetrics queries metrics-server for a pod's CPU and memory usage.
func (r *Runtime) GetPodMetrics(ctx context.Context, namespace, name string) (*PodResourceMetrics, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	pm, err := r.cachedPodMetrics(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	var totalCPUMilli, totalMemBytes int64
	for _, c := range pm.Containers {
		totalCPUMilli += c.Usage.Cpu().MilliValue()
		totalMemBytes += c.Usage.Memory().Value()
	}

	// Get memory limit from pod spec.
	var memLimit int64
	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		for _, c := range pod.Spec.Containers {
			if lim := c.Resources.Limits.Memory(); lim != nil {
				memLimit += lim.Value()
			}
		}
	}

	return &PodResourceMetrics{
		Name:          name,
		Namespace:     namespace,
		CPUMillicores: totalCPUMilli,
		MemBytes:      totalMemBytes,
		MemLimitBytes: memLimit,
		Timestamp:     pm.Timestamp.Time,
	}, nil
}

// GetNodeMetrics queries metrics-server for a node's CPU and memory usage.
func (r *Runtime) GetNodeMetrics(ctx context.Context, name string) (*NodeResourceMetrics, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	mc := r.metricsClient()
	if mc == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	nm, err := mc.MetricsV1beta1().NodeMetricses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node metrics %s: %w", name, err)
	}

	cpuMilli := nm.Usage.Cpu().MilliValue()
	memBytes := nm.Usage.Memory().Value()

	// Get capacity from node spec.
	var cpuCapacity, memCapacity int64
	node, err := cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if cpu := node.Status.Capacity.Cpu(); cpu != nil {
			cpuCapacity = cpu.MilliValue()
		}
		if mem := node.Status.Capacity.Memory(); mem != nil {
			memCapacity = mem.Value()
		}
	}

	return &NodeResourceMetrics{
		Name:                  name,
		CPUMillicores:         cpuMilli,
		CPUCapacityMillicores: cpuCapacity,
		MemBytes:              memBytes,
		MemCapacityBytes:      memCapacity,
		Timestamp:             nm.Timestamp.Time,
	}, nil
}
