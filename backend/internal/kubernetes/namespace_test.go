package kubernetes

import "testing"

func TestNamespaceFilter_DefaultExcludes(t *testing.T) {
	f := NewNamespaceFilter("", "")

	if f.IsAllowed("kube-system") {
		t.Error("kube-system should be excluded by default")
	}
	if f.IsAllowed("kube-public") {
		t.Error("kube-public should be excluded by default")
	}
	if f.IsAllowed("kube-node-lease") {
		t.Error("kube-node-lease should be excluded by default")
	}
	if !f.IsAllowed("default") {
		t.Error("default should be allowed")
	}
	if !f.IsAllowed("my-app") {
		t.Error("custom namespaces should be allowed")
	}
}

func TestNamespaceFilter_AllowlistOverridesExcludes(t *testing.T) {
	f := NewNamespaceFilter("staging,production", "")

	if !f.IsAllowed("staging") {
		t.Error("staging should be allowed")
	}
	if !f.IsAllowed("production") {
		t.Error("production should be allowed")
	}
	if f.IsAllowed("default") {
		t.Error("default should NOT be allowed when allowlist is set")
	}
	if f.IsAllowed("kube-system") {
		t.Error("kube-system should NOT be allowed when allowlist is set")
	}
}

func TestNamespaceFilter_CustomBlocklistAppendsDefaults(t *testing.T) {
	f := NewNamespaceFilter("", "monitoring,logging")

	if f.IsAllowed("kube-system") {
		t.Error("kube-system should still be excluded")
	}
	if f.IsAllowed("monitoring") {
		t.Error("monitoring should be excluded")
	}
	if f.IsAllowed("logging") {
		t.Error("logging should be excluded")
	}
	if !f.IsAllowed("default") {
		t.Error("default should be allowed")
	}
}

func TestNamespaceFilter_EmptyAllowsAllNonDefault(t *testing.T) {
	f := NewNamespaceFilter("", "")

	if !f.IsAllowed("any-namespace") {
		t.Error("arbitrary namespaces should be allowed")
	}
}
