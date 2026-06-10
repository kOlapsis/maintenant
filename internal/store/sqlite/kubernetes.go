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

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
)

// KubernetesStore persists per-agent Kubernetes topology (namespaces, workloads,
// pods, nodes). Each Replace*ForAgent reconciles the snapshot it is given,
// upserting present rows and hard-deleting the agent's rows that are gone.
type KubernetesStore struct {
	db     *sql.DB
	writer *Writer
}

// NewKubernetesStore creates a SQLite-backed Kubernetes topology store.
func NewKubernetesStore(d *DB) *KubernetesStore {
	return &KubernetesStore{db: d.ReadDB(), writer: d.Writer()}
}

// ReplaceNamespacesForAgent replaces the agent's namespace list wholesale.
// Namespaces carry no mutable fields beyond their key, so the snapshot fully
// supersedes the stored set: wipe the agent's rows, then insert the new set.
func (s *KubernetesStore) ReplaceNamespacesForAgent(ctx context.Context, agentID string, namespaces []string) error {
	agentID = uid.Agent(agentID)
	if _, err := s.writer.Exec(ctx, `DELETE FROM kubernetes_namespaces WHERE agent_id=?`, agentID); err != nil {
		return fmt.Errorf("clear namespaces for agent %s: %w", agentID, err)
	}
	for _, ns := range namespaces {
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO kubernetes_namespaces (id, agent_id, name) VALUES (?, ?, ?)`,
			uid.Namespace(agentID, ns), agentID, ns); err != nil {
			return fmt.Errorf("insert namespace %s: %w", ns, err)
		}
	}
	return nil
}

// ReplaceWorkloadsForAgent reconciles the agent's workloads.
func (s *KubernetesStore) ReplaceWorkloadsForAgent(ctx context.Context, agentID string, workloads []kubernetes.K8sWorkload) error {
	agentID = uid.Agent(agentID)
	keep := make([]string, 0, len(workloads))
	for i := range workloads {
		w := &workloads[i]
		id := uid.K8sWorkload(agentID, w.ID)
		keep = append(keep, id)
		images, err := json.Marshal(orEmptySlice(w.Images))
		if err != nil {
			return fmt.Errorf("marshal workload images %s: %w", w.ID, err)
		}
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO kubernetes_workloads (id, agent_id, workload_id, name, namespace, kind,
				images, ready_replicas, desired_replicas, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, workload_id) DO UPDATE SET
				name=excluded.name,
				namespace=excluded.namespace,
				kind=excluded.kind,
				images=excluded.images,
				ready_replicas=excluded.ready_replicas,
				desired_replicas=excluded.desired_replicas,
				status=excluded.status,
				created_at=excluded.created_at`,
			id, agentID, w.ID, w.Name, w.Namespace, w.Kind,
			string(images), w.ReadyReplicas, w.DesiredReplicas, w.Status, w.CreatedAt.Unix(),
		); err != nil {
			return fmt.Errorf("upsert workload %s: %w", w.ID, err)
		}
	}
	return deleteNotInForAgent(ctx, s.writer, "kubernetes_workloads", agentID, keep)
}

// ReplacePodsForAgent reconciles the agent's pods.
func (s *KubernetesStore) ReplacePodsForAgent(ctx context.Context, agentID string, pods []kubernetes.K8sPod) error {
	agentID = uid.Agent(agentID)
	keep := make([]string, 0, len(pods))
	for i := range pods {
		p := &pods[i]
		id := uid.Pod(agentID, p.Namespace, p.Name)
		keep = append(keep, id)
		containers, err := json.Marshal(orEmptyContainers(p.Containers))
		if err != nil {
			return fmt.Errorf("marshal pod containers %s/%s: %w", p.Namespace, p.Name, err)
		}
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO kubernetes_pods (id, agent_id, namespace, name, status, status_reason,
				restart_count, node_name, pod_ip, host_ip, workload_ref, containers, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, namespace, name) DO UPDATE SET
				status=excluded.status,
				status_reason=excluded.status_reason,
				restart_count=excluded.restart_count,
				node_name=excluded.node_name,
				pod_ip=excluded.pod_ip,
				host_ip=excluded.host_ip,
				workload_ref=excluded.workload_ref,
				containers=excluded.containers,
				created_at=excluded.created_at`,
			id, agentID, p.Namespace, p.Name, p.Status, p.StatusReason,
			p.RestartCount, p.NodeName, p.PodIP, p.HostIP, p.WorkloadRef, string(containers), p.CreatedAt.Unix(),
		); err != nil {
			return fmt.Errorf("upsert pod %s/%s: %w", p.Namespace, p.Name, err)
		}
	}
	return deleteNotInForAgent(ctx, s.writer, "kubernetes_pods", agentID, keep)
}

// ReplaceNodesForAgent reconciles the agent's nodes.
func (s *KubernetesStore) ReplaceNodesForAgent(ctx context.Context, agentID string, nodes []kubernetes.K8sNode) error {
	agentID = uid.Agent(agentID)
	keep := make([]string, 0, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		id := uid.K8sNode(agentID, n.Name)
		keep = append(keep, id)
		roles, err := json.Marshal(orEmptySlice(n.Roles))
		if err != nil {
			return fmt.Errorf("marshal node roles %s: %w", n.Name, err)
		}
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO kubernetes_nodes (id, agent_id, name, roles, status, running_pods,
				kubernetes_version, os_image, architecture,
				capacity_cpu_millicores, capacity_memory_bytes, capacity_pods,
				allocatable_cpu_millicores, allocatable_memory_bytes, allocatable_pods, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, name) DO UPDATE SET
				roles=excluded.roles,
				status=excluded.status,
				running_pods=excluded.running_pods,
				kubernetes_version=excluded.kubernetes_version,
				os_image=excluded.os_image,
				architecture=excluded.architecture,
				capacity_cpu_millicores=excluded.capacity_cpu_millicores,
				capacity_memory_bytes=excluded.capacity_memory_bytes,
				capacity_pods=excluded.capacity_pods,
				allocatable_cpu_millicores=excluded.allocatable_cpu_millicores,
				allocatable_memory_bytes=excluded.allocatable_memory_bytes,
				allocatable_pods=excluded.allocatable_pods,
				created_at=excluded.created_at`,
			id, agentID, n.Name, string(roles), n.Status, n.RunningPods,
			n.KubernetesVersion, n.OSImage, n.Architecture,
			n.Capacity.CPUMillicores, n.Capacity.MemoryBytes, n.Capacity.Pods,
			n.Allocatable.CPUMillicores, n.Allocatable.MemoryBytes, n.Allocatable.Pods, n.CreatedAt.Unix(),
		); err != nil {
			return fmt.Errorf("upsert k8s node %s: %w", n.Name, err)
		}
	}
	return deleteNotInForAgent(ctx, s.writer, "kubernetes_nodes", agentID, keep)
}

// ListNamespaces returns the agent's namespaces sorted by name. agentID empty
// returns every agent's namespaces (deduplicated).
func (s *KubernetesStore) ListNamespaces(ctx context.Context, agentID string) ([]string, error) {
	q := `SELECT DISTINCT name FROM kubernetes_namespaces`
	var rows *sql.Rows
	var err error
	if agentID != "" {
		q += ` WHERE agent_id=? ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q, uid.Agent(agentID))
	} else {
		q += ` ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, fmt.Errorf("list k8s namespaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan k8s namespace: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListWorkloads returns the agent's workloads grouped by namespace, optionally
// restricted to the given namespaces.
func (s *KubernetesStore) ListWorkloads(ctx context.Context, agentID string, namespaces []string) ([]kubernetes.K8sWorkloadGroup, error) {
	q := `SELECT agent_id, workload_id, name, namespace, kind, images, ready_replicas,
		desired_replicas, status, created_at FROM kubernetes_workloads`
	conds, args := agentNamespaceFilter(agentID, namespaces)
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY namespace ASC, kind ASC, name ASC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list k8s workloads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byNS := make(map[string][]kubernetes.K8sWorkload)
	var order []string
	for rows.Next() {
		var (
			w          kubernetes.K8sWorkload
			imagesJSON string
			createdAt  int64
		)
		if err := rows.Scan(&w.AgentID, &w.ID, &w.Name, &w.Namespace, &w.Kind, &imagesJSON,
			&w.ReadyReplicas, &w.DesiredReplicas, &w.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("scan k8s workload: %w", err)
		}
		_ = json.Unmarshal([]byte(imagesJSON), &w.Images)
		w.CreatedAt = time.Unix(createdAt, 0)
		if _, seen := byNS[w.Namespace]; !seen {
			order = append(order, w.Namespace)
		}
		byNS[w.Namespace] = append(byNS[w.Namespace], w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := make([]kubernetes.K8sWorkloadGroup, 0, len(order))
	for _, ns := range order {
		groups = append(groups, kubernetes.K8sWorkloadGroup{Namespace: ns, Workloads: byNS[ns]})
	}
	return groups, nil
}

// ListPods returns the agent's pods, optionally filtered by namespaces and the
// workload/node/status filters.
func (s *KubernetesStore) ListPods(ctx context.Context, agentID string, namespaces []string, filters kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	q := `SELECT agent_id, namespace, name, status, status_reason, restart_count, node_name,
		pod_ip, host_ip, workload_ref, containers, created_at FROM kubernetes_pods`
	conds, args := agentNamespaceFilter(agentID, namespaces)
	if filters.Node != "" {
		conds = append(conds, "node_name=?")
		args = append(args, filters.Node)
	}
	if filters.Status != "" {
		conds = append(conds, "status=?")
		args = append(args, filters.Status)
	}
	if filters.Workload != "" {
		conds = append(conds, "workload_ref LIKE ?")
		args = append(args, filters.Workload+"%")
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY namespace ASC, name ASC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list k8s pods: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []kubernetes.K8sPod
	for rows.Next() {
		var (
			p              kubernetes.K8sPod
			containersJSON string
			createdAt      int64
		)
		if err := rows.Scan(&p.AgentID, &p.Namespace, &p.Name, &p.Status, &p.StatusReason, &p.RestartCount,
			&p.NodeName, &p.PodIP, &p.HostIP, &p.WorkloadRef, &containersJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan k8s pod: %w", err)
		}
		_ = json.Unmarshal([]byte(containersJSON), &p.Containers)
		p.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplaceEventsForAgent replaces the agent's events wholesale. Events are
// ephemeral (the agent reports its current event window each snapshot), so the
// snapshot fully supersedes the stored set: wipe the agent's rows, then insert.
func (s *KubernetesStore) ReplaceEventsForAgent(ctx context.Context, agentID string, events []kubernetes.K8sEventRef) error {
	agentID = uid.Agent(agentID)
	if _, err := s.writer.Exec(ctx, `DELETE FROM kubernetes_events WHERE agent_id=?`, agentID); err != nil {
		return fmt.Errorf("clear k8s events for agent %s: %w", agentID, err)
	}
	for i := range events {
		e := &events[i]
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO kubernetes_events (id, agent_id, involved_kind, involved_namespace, involved_name,
				type, reason, message, source, first_seen, last_seen, count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid.New(), agentID, e.InvolvedKind, e.InvolvedNamespace, e.InvolvedName,
			e.Type, e.Reason, e.Message, e.Source, e.FirstSeen.Unix(), e.LastSeen.Unix(), e.Count,
		); err != nil {
			return fmt.Errorf("insert k8s event for %s/%s: %w", e.InvolvedNamespace, e.InvolvedName, err)
		}
	}
	return nil
}

// ListEventsForObject returns the agent's events concerning a single object
// (e.g. a Pod or a Deployment), newest first.
func (s *KubernetesStore) ListEventsForObject(ctx context.Context, agentID, kind, namespace, name string) ([]kubernetes.K8sEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT type, reason, message, source, first_seen, last_seen, count
		FROM kubernetes_events
		WHERE agent_id=? AND involved_kind=? AND involved_namespace=? AND involved_name=?
		ORDER BY last_seen DESC`,
		uid.Agent(agentID), kind, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("list k8s events for %s %s/%s: %w", kind, namespace, name, err)
	}
	defer func() { _ = rows.Close() }()

	var out []kubernetes.K8sEvent
	for rows.Next() {
		var (
			e                   kubernetes.K8sEvent
			firstSeen, lastSeen int64
		)
		if err := rows.Scan(&e.Type, &e.Reason, &e.Message, &e.Source, &firstSeen, &lastSeen, &e.Count); err != nil {
			return nil, fmt.Errorf("scan k8s event: %w", err)
		}
		e.FirstSeen = time.Unix(firstSeen, 0)
		e.LastSeen = time.Unix(lastSeen, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetWorkload returns a single workload by its natural id, or nil if absent.
func (s *KubernetesStore) GetWorkload(ctx context.Context, agentID, workloadID string) (*kubernetes.K8sWorkload, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workload_id, name, namespace, kind, images, ready_replicas,
			desired_replicas, status, created_at FROM kubernetes_workloads
		WHERE agent_id=? AND workload_id=?`, uid.Agent(agentID), workloadID)

	var (
		w          kubernetes.K8sWorkload
		imagesJSON string
		createdAt  int64
	)
	if err := row.Scan(&w.ID, &w.Name, &w.Namespace, &w.Kind, &imagesJSON,
		&w.ReadyReplicas, &w.DesiredReplicas, &w.Status, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get k8s workload %s: %w", workloadID, err)
	}
	_ = json.Unmarshal([]byte(imagesJSON), &w.Images)
	w.CreatedAt = time.Unix(createdAt, 0)
	return &w, nil
}

// GetPod returns a single pod by namespace and name, or nil if absent.
func (s *KubernetesStore) GetPod(ctx context.Context, agentID, namespace, name string) (*kubernetes.K8sPod, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT namespace, name, status, status_reason, restart_count, node_name,
			pod_ip, host_ip, workload_ref, containers, created_at FROM kubernetes_pods
		WHERE agent_id=? AND namespace=? AND name=?`, uid.Agent(agentID), namespace, name)

	var (
		p              kubernetes.K8sPod
		containersJSON string
		createdAt      int64
	)
	if err := row.Scan(&p.Namespace, &p.Name, &p.Status, &p.StatusReason, &p.RestartCount,
		&p.NodeName, &p.PodIP, &p.HostIP, &p.WorkloadRef, &containersJSON, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get k8s pod %s/%s: %w", namespace, name, err)
	}
	_ = json.Unmarshal([]byte(containersJSON), &p.Containers)
	p.CreatedAt = time.Unix(createdAt, 0)
	return &p, nil
}

// ListNodes returns the agent's nodes sorted by name.
func (s *KubernetesStore) ListNodes(ctx context.Context, agentID string) ([]kubernetes.K8sNode, error) {
	q := `SELECT name, roles, status, running_pods, kubernetes_version, os_image, architecture,
		capacity_cpu_millicores, capacity_memory_bytes, capacity_pods,
		allocatable_cpu_millicores, allocatable_memory_bytes, allocatable_pods, created_at
		FROM kubernetes_nodes`
	var rows *sql.Rows
	var err error
	if agentID != "" {
		q += ` WHERE agent_id=? ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q, uid.Agent(agentID))
	} else {
		q += ` ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, fmt.Errorf("list k8s nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []kubernetes.K8sNode
	for rows.Next() {
		var (
			n         kubernetes.K8sNode
			rolesJSON string
			createdAt int64
		)
		if err := rows.Scan(&n.Name, &rolesJSON, &n.Status, &n.RunningPods,
			&n.KubernetesVersion, &n.OSImage, &n.Architecture,
			&n.Capacity.CPUMillicores, &n.Capacity.MemoryBytes, &n.Capacity.Pods,
			&n.Allocatable.CPUMillicores, &n.Allocatable.MemoryBytes, &n.Allocatable.Pods, &createdAt); err != nil {
			return nil, fmt.Errorf("scan k8s node: %w", err)
		}
		_ = json.Unmarshal([]byte(rolesJSON), &n.Roles)
		n.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, n)
	}
	return out, rows.Err()
}

// agentNamespaceFilter builds the shared agent_id + namespace IN (...) WHERE
// conditions used by the workload and pod queries.
func agentNamespaceFilter(agentID string, namespaces []string) ([]string, []interface{}) {
	var conds []string
	var args []interface{}
	if agentID != "" {
		conds = append(conds, "agent_id=?")
		args = append(args, uid.Agent(agentID))
	}
	if len(namespaces) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(namespaces)), ",")
		conds = append(conds, "namespace IN ("+ph+")")
		for _, ns := range namespaces {
			args = append(args, ns)
		}
	}
	return conds, args
}

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyContainers(c []kubernetes.K8sContainerStatus) []kubernetes.K8sContainerStatus {
	if c == nil {
		return []kubernetes.K8sContainerStatus{}
	}
	return c
}
