package kubernetes

import (
	"context"
	"log/slog"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool   { return &b }

func TestDiscoverAll_Deployments(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.25",
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 3,
		},
	}

	cs := fake.NewClientset(dep)
	rt := &Runtime{
		logger:    slog.Default(),
		nsFilter:  NewNamespaceFilter("", ""),
		clientset: cs,
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	containers, err := rt.discoverAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ExternalID == "default/Deployment/nginx" {
			found = true
			if c.RuntimeType != "kubernetes" {
				t.Errorf("expected RuntimeType=kubernetes, got %s", c.RuntimeType)
			}
			if c.ControllerKind != "Deployment" {
				t.Errorf("expected ControllerKind=Deployment, got %s", c.ControllerKind)
			}
			if c.OrchestrationGroup != "default" {
				t.Errorf("expected OrchestrationGroup=default, got %s", c.OrchestrationGroup)
			}
			if c.OrchestrationUnit != "nginx" {
				t.Errorf("expected OrchestrationUnit=nginx, got %s", c.OrchestrationUnit)
			}
			if c.PodCount != 3 {
				t.Errorf("expected PodCount=3, got %d", c.PodCount)
			}
			if c.ReadyCount != 3 {
				t.Errorf("expected ReadyCount=3, got %d", c.ReadyCount)
			}
			if c.Image != "nginx:1.25" {
				t.Errorf("expected Image=nginx:1.25, got %s", c.Image)
			}
			if c.State != "running" {
				t.Errorf("expected State=running, got %s", c.State)
			}
		}
	}
	if !found {
		t.Error("deployment not found in discovered containers")
	}
}

func TestDiscoverAll_NamespaceFiltering(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-server",
			Namespace: "kube-system",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: "metrics-server:latest"}},
				},
			},
		},
	}

	cs := fake.NewClientset(dep)
	rt := &Runtime{
		logger:    slog.Default(),
		nsFilter:  NewNamespaceFilter("", ""),
		clientset: cs,
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	containers, err := rt.discoverAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range containers {
		if c.Namespace == "kube-system" {
			t.Error("kube-system workloads should be filtered out")
		}
	}
}

func TestDiscoverAll_BarePods(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "debug-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "debug",
				Image: "busybox:latest",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	cs := fake.NewClientset(pod)
	rt := &Runtime{
		logger:    slog.Default(),
		nsFilter:  NewNamespaceFilter("", ""),
		clientset: cs,
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	containers, err := rt.discoverAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ExternalID == "default/debug-pod" {
			found = true
			if c.ControllerKind != "" {
				t.Errorf("bare pod should have empty ControllerKind, got %s", c.ControllerKind)
			}
			if c.State != "running" {
				t.Errorf("expected running, got %s", c.State)
			}
		}
	}
	if !found {
		t.Error("bare pod not found in discovered containers")
	}
}

func TestDiscoverAll_ManagedPodsExcluded(t *testing.T) {
	// Pod owned by a ReplicaSet (managed by Deployment)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "ReplicaSet",
				Controller: boolPtr(true),
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Image: "nginx:1.25"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cs := fake.NewClientset(pod)
	rt := &Runtime{
		logger:    slog.Default(),
		nsFilter:  NewNamespaceFilter("", ""),
		clientset: cs,
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	containers, err := rt.discoverAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range containers {
		if c.ExternalID == "default/nginx-abc123" {
			t.Error("managed pods should not appear as standalone workloads")
		}
	}
}

func TestDiscoverAll_Annotations(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api",
			Namespace: "default",
			Annotations: map[string]string{
				"pulseboard.group":                "backend",
				"pulseboard.alert.severity":       "critical",
				"pulseboard.alert.restart_threshold": "5",
				"pulseboard.alert.channels":       "slack",
				"pulseboard.ignore":               "true",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: "api:v1"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}

	cs := fake.NewClientset(dep)
	rt := &Runtime{
		logger:    slog.Default(),
		nsFilter:  NewNamespaceFilter("", ""),
		clientset: cs,
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	containers, err := rt.discoverAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range containers {
		if c.ExternalID == "default/Deployment/api" {
			if c.CustomGroup != "backend" {
				t.Errorf("expected CustomGroup=backend, got %s", c.CustomGroup)
			}
			if c.AlertSeverity != "critical" {
				t.Errorf("expected AlertSeverity=critical, got %s", c.AlertSeverity)
			}
			if c.RestartThreshold != 5 {
				t.Errorf("expected RestartThreshold=5, got %d", c.RestartThreshold)
			}
			if c.AlertChannels != "slack" {
				t.Errorf("expected AlertChannels=slack, got %s", c.AlertChannels)
			}
			if !c.IsIgnored {
				t.Error("expected IsIgnored=true")
			}
			return
		}
	}
	t.Error("annotated deployment not found")
}

func TestExternalIDFormat(t *testing.T) {
	tests := []struct {
		id   string
		ns   string
		kind string
		name string
		err  bool
	}{
		{"default/Deployment/nginx", "default", "Deployment", "nginx", false},
		{"prod/StatefulSet/postgres", "prod", "StatefulSet", "postgres", false},
		{"default/debug-pod", "default", "", "debug-pod", false},
		{"bad", "", "", "", true},
	}

	for _, tt := range tests {
		ns, kind, name, err := parseExternalID(tt.id)
		if tt.err {
			if err == nil {
				t.Errorf("expected error for %q", tt.id)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for %q: %v", tt.id, err)
			continue
		}
		if ns != tt.ns || kind != tt.kind || name != tt.name {
			t.Errorf("parseExternalID(%q) = (%s, %s, %s), want (%s, %s, %s)", tt.id, ns, kind, name, tt.ns, tt.kind, tt.name)
		}
	}
}
