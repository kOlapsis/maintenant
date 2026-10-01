// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	clientCallRe   = regexp.MustCompile(`\.(CoreV1|AppsV1|BatchV1|MetricsV1beta1)\(\)\.(\w+)\([^()]*\)\.(\w+)\(`)
	informerCallRe = regexp.MustCompile(`\.(Core|Apps|Batch)\(\)\.V1\(\)\.(\w+)\(\)\.Informer\(\)`)
)

var apiGroupOf = map[string]string{
	"CoreV1": "", "AppsV1": "apps", "BatchV1": "batch", "MetricsV1beta1": "metrics.k8s.io",
	"Core": "", "Apps": "apps", "Batch": "batch",
}

var metricsResource = map[string]string{"podmetricses": "pods", "nodemetricses": "nodes"}

func grant(group, resource, verb string) string {
	return group + "|" + resource + "|" + verb
}

func grantsOf(rules []rbacv1.PolicyRule) map[string]bool {
	out := map[string]bool{}
	for _, r := range rules {
		for _, g := range r.APIGroups {
			for _, res := range r.Resources {
				for _, v := range r.Verbs {
					out[grant(g, res, v)] = true
				}
			}
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// apiCallsInPackage reads the package sources and returns every (group, resource, verb) they call.
func apiCallsInPackage(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
			group, resource := apiGroupOf[m[1]], strings.ToLower(m[2])
			if r, ok := metricsResource[resource]; ok {
				resource = r
			}
			switch m[3] {
			case "Get", "List", "Watch":
				calls[grant(group, resource, strings.ToLower(m[3]))] = true
			case "GetLogs":
				calls[grant(group, resource+"/log", "get")] = true
			default:
				t.Errorf("%s: %s().%s().%s is not a read call", f, m[1], m[2], m[3])
			}
		}
		for _, m := range informerCallRe.FindAllStringSubmatch(string(src), -1) {
			group, resource := apiGroupOf[m[1]], strings.ToLower(m[2])
			calls[grant(group, resource, "list")] = true
			calls[grant(group, resource, "watch")] = true
		}
	}
	return calls
}

func TestReadRulesCoverEveryAPICall(t *testing.T) {
	calls := apiCallsInPackage(t)
	if !calls[grant("", "pods", "list")] || !calls[grant("metrics.k8s.io", "nodes", "get")] {
		t.Fatalf("source scan found too little, the patterns no longer match the client calls: %v", sortedKeys(calls))
	}
	granted := grantsOf(ReadRules())
	for _, c := range sortedKeys(calls) {
		if !granted[c] {
			t.Errorf("the runtime calls %s but ReadRules does not grant it", c)
		}
	}
}

func TestReadRulesAreReadOnly(t *testing.T) {
	for _, r := range ReadRules() {
		for _, v := range r.Verbs {
			if v != "get" && v != "list" && v != "watch" {
				t.Errorf("rule %v grants %q", r.Resources, v)
			}
		}
	}
}

type manifestObject struct {
	Kind     string              `json:"kind"`
	Metadata metav1.ObjectMeta   `json:"metadata"`
	Rules    []rbacv1.PolicyRule `json:"rules"`
	Subjects []rbacv1.Subject    `json:"subjects"`
}

func decodeManifests(t *testing.T, data []byte) []manifestObject {
	t.Helper()
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var objs []manifestObject
	for {
		var obj manifestObject
		err := dec.Decode(&obj)
		if errors.Is(err, io.EOF) {
			return objs
		}
		if err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		if obj.Kind != "" {
			objs = append(objs, obj)
		}
	}
}

func readManifest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// withoutTemplateLines drops the Helm action lines so the chart's static rules parse as YAML.
func withoutTemplateLines(data []byte) []byte {
	var out [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if !bytes.Contains(line, []byte("{{")) {
			out = append(out, line)
		}
	}
	return bytes.Join(out, []byte("\n"))
}

func TestServerManifestsGrantExactlyReadRules(t *testing.T) {
	sources := map[string][]byte{
		"deploy/kubernetes/rbac.yaml":                readManifest(t, "deploy/kubernetes/rbac.yaml"),
		"deploy/helm/maintenant/templates/rbac.yaml": withoutTemplateLines(readManifest(t, "deploy/helm/maintenant/templates/rbac.yaml")),
	}
	want := grantsOf(ReadRules())
	for path, data := range sources {
		var roles int
		for _, obj := range decodeManifests(t, data) {
			if obj.Kind != "ClusterRole" {
				continue
			}
			roles++
			got := grantsOf(obj.Rules)
			for _, g := range sortedKeys(want) {
				if !got[g] {
					t.Errorf("%s: ClusterRole misses %s", path, g)
				}
			}
			for _, g := range sortedKeys(got) {
				if !want[g] {
					t.Errorf("%s: ClusterRole grants %s, which the runtime never uses", path, g)
				}
			}
		}
		if roles != 1 {
			t.Errorf("%s: %d ClusterRole, want 1", path, roles)
		}
	}
}

func TestPlainManifestsLiveInTheMaintenantNamespace(t *testing.T) {
	const namespace = "maintenant"
	clusterScoped := map[string]bool{"ClusterRole": true, "ClusterRoleBinding": true, "Namespace": true}
	for _, path := range []string{"deploy/kubernetes/deployment.yaml", "deploy/kubernetes/rbac.yaml"} {
		for _, obj := range decodeManifests(t, readManifest(t, path)) {
			if clusterScoped[obj.Kind] {
				if obj.Metadata.Namespace != "" {
					t.Errorf("%s: cluster-scoped %s %q carries a namespace", path, obj.Kind, obj.Metadata.Name)
				}
			} else if obj.Metadata.Namespace != namespace {
				t.Errorf("%s: %s %q is in namespace %q, want %q", path, obj.Kind, obj.Metadata.Name, obj.Metadata.Namespace, namespace)
			}
			for _, s := range obj.Subjects {
				if s.Kind == "ServiceAccount" && s.Namespace != namespace {
					t.Errorf("%s: %s binds ServiceAccount %s/%s, want namespace %q", path, obj.Metadata.Name, s.Namespace, s.Name, namespace)
				}
			}
		}
	}
}
