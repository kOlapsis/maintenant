// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package kubernetes

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// GetPodMetrics queries metrics-server for a pod's CPU and memory usage.
func (r *Runtime) GetPodMetrics(ctx context.Context, namespace, name string) (*PodResourceMetrics, error) {
	if r.metrics == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	pm, err := r.metrics.MetricsV1beta1().PodMetricses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod metrics %s/%s: %w", namespace, name, err)
	}

	var totalCPUMilli, totalMemBytes int64
	for _, c := range pm.Containers {
		totalCPUMilli += c.Usage.Cpu().MilliValue()
		totalMemBytes += c.Usage.Memory().Value()
	}

	// Get memory limit from pod spec.
	var memLimit int64
	pod, err := r.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
	if r.metrics == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	nm, err := r.metrics.MetricsV1beta1().NodeMetricses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node metrics %s: %w", name, err)
	}

	cpuMilli := nm.Usage.Cpu().MilliValue()
	memBytes := nm.Usage.Memory().Value()

	// Get capacity from node spec.
	var cpuCapacity, memCapacity int64
	node, err := r.clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
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
