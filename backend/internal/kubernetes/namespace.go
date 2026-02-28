package kubernetes

import "strings"

// Default namespaces excluded from monitoring.
var defaultExcluded = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// NamespaceFilter controls which namespaces PulseBoard monitors.
type NamespaceFilter struct {
	allowlist map[string]bool // if non-empty, ONLY these namespaces are allowed
	blocklist map[string]bool // merged with defaults; checked when allowlist is empty
}

// NewNamespaceFilter creates a filter from env var values.
// allowCSV = PULSEBOARD_K8S_NAMESPACES (comma-separated allowlist).
// excludeCSV = PULSEBOARD_K8S_EXCLUDE_NAMESPACES (comma-separated blocklist, appended to defaults).
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
