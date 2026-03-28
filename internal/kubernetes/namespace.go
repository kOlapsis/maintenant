// Copyright 2026 Benjamin Touchard (Kolapsis)
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Default namespaces excluded from monitoring.
var defaultExcluded = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// NamespaceFilter controls which namespaces maintenant monitors.
type NamespaceFilter struct {
	allowlist map[string]bool // if non-empty, ONLY these namespaces are allowed
	blocklist map[string]bool // merged with defaults; checked when allowlist is empty
}

// NewNamespaceFilter creates a filter from env var values.
// allowCSV = MAINTENANT_K8S_NAMESPACES (comma-separated allowlist).
// excludeCSV = MAINTENANT_K8S_EXCLUDE_NAMESPACES (comma-separated blocklist, appended to defaults).
func NewNamespaceFilter(allowCSV, excludeCSV string) *NamespaceFilter {
	f := &NamespaceFilter{
		allowlist: map[string]bool{},
		blocklist: map[string]bool{},
	}

	if allowCSV != "" {
		for _, ns := range strings.Split(allowCSV, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				f.allowlist[ns] = true
			}
		}
	}

	if len(f.allowlist) == 0 {
		// Apply default excludes + custom blocklist.
		for ns := range defaultExcluded {
			f.blocklist[ns] = true
		}
		if excludeCSV != "" {
			for _, ns := range strings.Split(excludeCSV, ",") {
				ns = strings.TrimSpace(ns)
				if ns != "" {
					f.blocklist[ns] = true
				}
			}
		}
	}

	return f
}

// IsAllowed returns true if the namespace should be monitored.
func (f *NamespaceFilter) IsAllowed(namespace string) bool {
	if len(f.allowlist) > 0 {
		return f.allowlist[namespace]
	}
	return !f.blocklist[namespace]
}

// ListNamespaces returns the allowed namespace names from the cluster.
// The result respects the allowlist/blocklist configured via env vars.
func (r *Runtime) ListNamespaces(ctx context.Context) ([]string, error) {
	nsList, err := r.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	var result []string
	for i := range nsList.Items {
		name := nsList.Items[i].Name
		if r.nsFilter.IsAllowed(name) {
			result = append(result, name)
		}
	}

	sort.Strings(result)
	return result, nil
}
