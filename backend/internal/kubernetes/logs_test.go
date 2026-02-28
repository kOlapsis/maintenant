package kubernetes

import (
	"context"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResolveLogTarget_PodLevel(t *testing.T) {
	cs := fake.NewClientset()
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	ns, pod, container, err := rt.resolveLogTarget(context.Background(), "default/my-pod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ns != "default" {
		t.Errorf("expected ns=default, got %s", ns)
	}
	if pod != "my-pod" {
		t.Errorf("expected pod=my-pod, got %s", pod)
	}
	if container != "" {
		t.Errorf("expected empty container, got %s", container)
	}
}

func TestResolveLogTarget_PodWithContainer(t *testing.T) {
	cs := fake.NewClientset()
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	ns, pod, container, err := rt.resolveLogTarget(context.Background(), "default/my-pod/sidecar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ns != "default" || pod != "my-pod" || container != "sidecar" {
		t.Errorf("got (%s, %s, %s), want (default, my-pod, sidecar)", ns, pod, container)
	}
}

func TestResolveLogTarget_ControllerResolvesToPod(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "prod",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "web"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "web", Image: "web:v1"}},
				},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc123",
			Namespace: "prod",
			Labels:    map[string]string{"app": "web"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cs := fake.NewClientset(dep, pod)
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	ns, podName, container, err := rt.resolveLogTarget(context.Background(), "prod/Deployment/web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ns != "prod" {
		t.Errorf("expected ns=prod, got %s", ns)
	}
	if podName != "web-abc123" {
		t.Errorf("expected pod=web-abc123, got %s", podName)
	}
	if container != "" {
		t.Errorf("expected empty container, got %s", container)
	}
}

func TestResolveLogTarget_ControllerWithContainer(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "prod",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "web"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "web", Image: "web:v1"},
						{Name: "sidecar", Image: "proxy:v1"},
					},
				},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-xyz789",
			Namespace: "prod",
			Labels:    map[string]string{"app": "web"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cs := fake.NewClientset(dep, pod)
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	ns, podName, container, err := rt.resolveLogTarget(context.Background(), "prod/Deployment/web/sidecar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ns != "prod" || podName != "web-xyz789" || container != "sidecar" {
		t.Errorf("got (%s, %s, %s), want (prod, web-xyz789, sidecar)", ns, podName, container)
	}
}

func TestResolveLogTarget_InvalidFormat(t *testing.T) {
	cs := fake.NewClientset()
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	_, _, _, err := rt.resolveLogTarget(context.Background(), "invalid")
	if err == nil {
		t.Error("expected error for invalid externalID")
	}
}

func TestResolveLogTarget_NoPods(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ghost",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(0),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "ghost"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "ghost"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "ghost", Image: "ghost:v1"}},
				},
			},
		},
	}

	cs := fake.NewClientset(dep)
	rt := &Runtime{
		logger:    slog.Default(),
		clientset: cs,
		nsFilter:  NewNamespaceFilter("", ""),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}

	_, _, _, err := rt.resolveLogTarget(context.Background(), "default/Deployment/ghost")
	if err == nil {
		t.Error("expected error when no pods available")
	}
}
