package kubernetes

import (
	"context"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPodHealth_Running_AllReady(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:           "web",
				ReadinessProbe: &corev1.Probe{},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Ready:        true,
				RestartCount: 0,
			}},
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

	hi, err := rt.podHealth(context.Background(), "default", "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hi.HasHealthCheck {
		t.Error("expected HasHealthCheck=true")
	}
	if hi.Status != "healthy" {
		t.Errorf("expected healthy, got %s", hi.Status)
	}
}

func TestPodHealth_ProbeFailure(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:           "web",
				ReadinessProbe: &corev1.Probe{},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Ready:        false,
				RestartCount: 3,
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: "back-off 5m0s restarting failed container",
					},
				},
			}},
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

	hi, err := rt.podHealth(context.Background(), "default", "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hi.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %s", hi.Status)
	}
	if hi.FailingStreak != 3 {
		t.Errorf("expected FailingStreak=3, got %d", hi.FailingStreak)
	}
}

func TestPodHealth_NoProbe(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "worker"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Ready: true,
			}},
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

	hi, err := rt.podHealth(context.Background(), "default", "worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hi.HasHealthCheck {
		t.Error("expected HasHealthCheck=false for pod without probes")
	}
	if hi.Status != "none" {
		t.Errorf("expected none, got %s", hi.Status)
	}
}

func TestPodHealth_Pending(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:           "web",
				ReadinessProbe: &corev1.Probe{},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
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

	hi, err := rt.podHealth(context.Background(), "default", "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hi.Status != "starting" {
		t.Errorf("expected starting, got %s", hi.Status)
	}
}
