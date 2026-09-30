// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import rbacv1 "k8s.io/api/rbac/v1"

// ReadRules returns the read-only ClusterRole rules covering every API call the runtime makes.
func ReadRules() []rbacv1.PolicyRule {
	read := []string{"get", "list", "watch"}
	return []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"namespaces", "nodes", "pods", "events", "services"}, Verbs: read},
		{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}, Verbs: read},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: read},
		{APIGroups: []string{"metrics.k8s.io"}, Resources: []string{"pods", "nodes"}, Verbs: []string{"get", "list"}},
	}
}
