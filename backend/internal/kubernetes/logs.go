package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fetchLogs retrieves the last N lines of logs from a pod.
// externalID format: "namespace/pod-name[/container-name]" or "namespace/Kind/name[/container-name]".
func (r *Runtime) fetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error) {
	ns, podName, containerName, err := r.resolveLogTarget(ctx, externalID)
	if err != nil {
		return nil, err
	}

	tailLines := int64(lines)
	opts := &corev1.PodLogOptions{
		TailLines:  &tailLines,
		Timestamps: timestamps,
	}
	if containerName != "" {
		opts.Container = containerName
	}

	stream, err := r.clientset.CoreV1().Pods(ns).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("get logs %s/%s: %w", ns, podName, err)
	}
	defer stream.Close()

	var result []string
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		result = append(result, scanner.Text())
	}
	return result, scanner.Err()
}

// streamLogs returns a streaming reader for pod logs.
func (r *Runtime) streamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error) {
	ns, podName, containerName, err := r.resolveLogTarget(ctx, externalID)
	if err != nil {
		return nil, err
	}

	tailLines := int64(lines)
	opts := &corev1.PodLogOptions{
		Follow:     true,
		TailLines:  &tailLines,
		Timestamps: timestamps,
	}
	if containerName != "" {
		opts.Container = containerName
	}

	stream, err := r.clientset.CoreV1().Pods(ns).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("stream logs %s/%s: %w", ns, podName, err)
	}

	return stream, nil
}

// resolveLogTarget resolves an externalID to (namespace, podName, containerName).
// For controller-level IDs, picks the first running pod.
func (r *Runtime) resolveLogTarget(ctx context.Context, externalID string) (ns, podName, containerName string, err error) {
	// Check if there's a container name suffix (4th segment).
	parts := strings.Split(externalID, "/")

	switch len(parts) {
	case 2:
		// namespace/pod-name
		return parts[0], parts[1], "", nil
	case 3:
		// Could be namespace/Kind/name (controller) or namespace/pod-name/container-name
		if isControllerKind(parts[1]) {
			// Controller: resolve to a running pod.
			pod, err := r.findActivePod(ctx, parts[0], parts[1], parts[2])
			if err != nil {
				return "", "", "", err
			}
			return parts[0], pod, "", nil
		}
		// namespace/pod-name/container-name
		return parts[0], parts[1], parts[2], nil
	case 4:
		// namespace/Kind/name/container-name
		pod, err := r.findActivePod(ctx, parts[0], parts[1], parts[2])
		if err != nil {
			return "", "", "", err
		}
		return parts[0], pod, parts[3], nil
	default:
		return "", "", "", fmt.Errorf("invalid log target: %q", externalID)
	}
}

func isControllerKind(s string) bool {
	switch s {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	}
	return false
}

// findActivePod resolves a controller to one of its running pods.
func (r *Runtime) findActivePod(ctx context.Context, ns, kind, name string) (string, error) {
	selector, err := r.controllerSelector(ctx, ns, kind, name)
	if err != nil {
		return "", err
	}

	podList, err := r.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", fmt.Errorf("list pods for %s/%s/%s: %w", ns, kind, name, err)
	}

	// Prefer a running pod.
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodRunning {
			return podList.Items[i].Name, nil
		}
	}
	// Fall back to any pod.
	if len(podList.Items) > 0 {
		return podList.Items[0].Name, nil
	}

	return "", fmt.Errorf("no pods found for %s/%s/%s", ns, kind, name)
}
