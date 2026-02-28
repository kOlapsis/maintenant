package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// statsSnapshot queries metrics-server for a workload's CPU and memory.
// externalID format: "namespace/ControllerKind/name" or "namespace/pod-name".
func (r *Runtime) statsSnapshot(ctx context.Context, externalID string) (*pbruntime.RawStats, error) {
	if r.metrics == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	ns, kind, name, err := parseExternalID(externalID)
	if err != nil {
		return nil, err
	}

	// For controller-level IDs, aggregate across pods.
	if kind != "" {
		return r.controllerStats(ctx, ns, kind, name)
	}

	// Pod-level: direct query.
	return r.podStats(ctx, ns, name)
}

func (r *Runtime) podStats(ctx context.Context, ns, podName string) (*pbruntime.RawStats, error) {
	pm, err := r.metrics.MetricsV1beta1().PodMetricses(ns).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod metrics %s/%s: %w", ns, podName, err)
	}

	var totalCPUMilli int64
	var totalMemBytes int64
	for _, c := range pm.Containers {
		totalCPUMilli += c.Usage.Cpu().MilliValue()
		totalMemBytes += c.Usage.Memory().Value()
	}

	cpuPercent := r.computeCPUPercent(pm.Name, totalCPUMilli, pm.Timestamp.Time)

	// Get memory limit from pod spec.
	pod, err := r.clientset.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
	var memLimit int64
	if err == nil {
		for _, c := range pod.Spec.Containers {
			if lim := c.Resources.Limits.Memory(); lim != nil {
				memLimit += lim.Value()
			}
		}
	}

	return &pbruntime.RawStats{
		CPUPercent:      cpuPercent,
		MemUsed:         totalMemBytes,
		MemLimit:        memLimit,
		NetRxBytes:      -1,
		NetTxBytes:      -1,
		BlockReadBytes:  -1,
		BlockWriteBytes: -1,
		Timestamp:       pm.Timestamp.Time,
	}, nil
}

func (r *Runtime) controllerStats(ctx context.Context, ns, kind, name string) (*pbruntime.RawStats, error) {
	// Build label selector from controller spec.
	selector, err := r.controllerSelector(ctx, ns, kind, name)
	if err != nil {
		return nil, err
	}

	podList, err := r.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s/%s/%s: %w", ns, kind, name, err)
	}

	var totalCPUMilli, totalMemBytes, totalMemLimit int64
	for _, pod := range podList.Items {
		pm, err := r.metrics.MetricsV1beta1().PodMetricses(ns).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			continue // pod might not have metrics yet
		}
		for _, c := range pm.Containers {
			totalCPUMilli += c.Usage.Cpu().MilliValue()
			totalMemBytes += c.Usage.Memory().Value()
		}
		for _, c := range pod.Spec.Containers {
			if lim := c.Resources.Limits.Memory(); lim != nil {
				totalMemLimit += lim.Value()
			}
		}
	}

	externalID := fmt.Sprintf("%s/%s/%s", ns, kind, name)
	cpuPercent := r.computeCPUPercent(externalID, totalCPUMilli, time.Now())

	return &pbruntime.RawStats{
		CPUPercent:      cpuPercent,
		MemUsed:         totalMemBytes,
		MemLimit:        totalMemLimit,
		NetRxBytes:      -1,
		NetTxBytes:      -1,
		BlockReadBytes:  -1,
		BlockWriteBytes: -1,
		Timestamp:       time.Now(),
	}, nil
}

// computeCPUPercent converts milliCPU to percentage using delta computation.
func (r *Runtime) computeCPUPercent(key string, milliCPU int64, ts time.Time) float64 {
	r.mu.Lock()
	prev, hasPrev := r.prevCPU[key]
	r.prevCPU[key] = &cpuPrev{milliCPU: milliCPU, timestamp: ts}
	r.mu.Unlock()

	if !hasPrev {
		// First sample: estimate from milliCPU (1000m = 100% of 1 core).
		return float64(milliCPU) / 10.0
	}

	elapsed := ts.Sub(prev.timestamp).Seconds()
	if elapsed <= 0 {
		return float64(milliCPU) / 10.0
	}

	// milliCPU is an instantaneous rate from metrics-server window.
	// Just convert directly: 1000 milliCPU = 1 core = 100% of 1 core.
	return float64(milliCPU) / 10.0
}

func (r *Runtime) controllerSelector(ctx context.Context, ns, kind, name string) (string, error) {
	switch kind {
	case "Deployment":
		dep, err := r.clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get deployment %s/%s: %w", ns, name, err)
		}
		if dep.Spec.Selector != nil {
			return labels.Set(dep.Spec.Selector.MatchLabels).String(), nil
		}
	case "StatefulSet":
		ss, err := r.clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get statefulset %s/%s: %w", ns, name, err)
		}
		if ss.Spec.Selector != nil {
			return labels.Set(ss.Spec.Selector.MatchLabels).String(), nil
		}
	case "DaemonSet":
		ds, err := r.clientset.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get daemonset %s/%s: %w", ns, name, err)
		}
		if ds.Spec.Selector != nil {
			return labels.Set(ds.Spec.Selector.MatchLabels).String(), nil
		}
	}
	return "", fmt.Errorf("unsupported controller kind: %s", kind)
}

// parseExternalID splits an externalID into namespace, kind, and name.
// Formats: "namespace/Kind/name" (controller) or "namespace/pod-name" (bare pod).
func parseExternalID(id string) (ns, kind, name string, err error) {
	parts := strings.SplitN(id, "/", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], nil
	case 2:
		return parts[0], "", parts[1], nil
	default:
		return "", "", "", fmt.Errorf("invalid externalID format: %q (expected namespace/name or namespace/Kind/name)", id)
	}
}
