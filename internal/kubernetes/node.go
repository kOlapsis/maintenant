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
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8sNode represents a Kubernetes cluster node.
type K8sNode struct {
	Name              string
	Roles             []string
	Conditions        []K8sCondition
	Status            string // ready, not-ready, unknown
	Capacity          K8sResourceQuantity
	Allocatable       K8sResourceQuantity
	RunningPods       int
	KubernetesVersion string
	OSImage           string
	Architecture      string
	CreatedAt         time.Time
}

// K8sResourceQuantity holds parsed resource capacity values.
type K8sResourceQuantity struct {
	CPUMillicores int64
	MemoryBytes   int64
	Pods          int64
}

// ListNodes returns all cluster nodes with their resource capacity and status.
func (r *Runtime) ListNodes(ctx context.Context) ([]K8sNode, error) {
	nodeList, err := r.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	nodes := make([]K8sNode, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes = append(nodes, mapNode(&nodeList.Items[i]))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	return nodes, nil
}

func mapNode(n *corev1.Node) K8sNode {
	conditions := make([]K8sCondition, 0, len(n.Status.Conditions))
	for _, c := range n.Status.Conditions {
		conditions = append(conditions, K8sCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
	}

	return K8sNode{
		Name:              n.Name,
		Roles:             nodeRoles(n),
		Conditions:        conditions,
		Status:            nodeStatus(n),
		Capacity:          mapResourceQuantity(n.Status.Capacity),
		Allocatable:       mapResourceQuantity(n.Status.Allocatable),
		KubernetesVersion: n.Status.NodeInfo.KubeletVersion,
		OSImage:           n.Status.NodeInfo.OSImage,
		Architecture:      n.Status.NodeInfo.Architecture,
		CreatedAt:         n.CreationTimestamp.Time,
	}
}

// nodeRoles extracts roles from standard node labels (node-role.kubernetes.io/<role>).
func nodeRoles(n *corev1.Node) []string {
	const prefix = "node-role.kubernetes.io/"
	var roles []string
	for label := range n.Labels {
		if strings.HasPrefix(label, prefix) {
			role := strings.TrimPrefix(label, prefix)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		roles = []string{"worker"}
	}
	sort.Strings(roles)
	return roles
}

// nodeStatus returns a simplified node status string.
func nodeStatus(n *corev1.Node) string {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			switch c.Status {
			case corev1.ConditionTrue:
				return "ready"
			case corev1.ConditionFalse:
				return "not-ready"
			default:
				return "unknown"
			}
		}
	}
	return "unknown"
}

// mapResourceQuantity converts a ResourceList to K8sResourceQuantity.
func mapResourceQuantity(rl corev1.ResourceList) K8sResourceQuantity {
	var q K8sResourceQuantity

	if cpu, ok := rl[corev1.ResourceCPU]; ok {
		q.CPUMillicores = cpu.MilliValue()
	}
	if mem, ok := rl[corev1.ResourceMemory]; ok {
		q.MemoryBytes = mem.Value()
	}
	if pods, ok := rl[corev1.ResourcePods]; ok {
		q.Pods = pods.Value()
	}

	return q
}
