package runtime

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/kolapsis/pulseboard/internal/container"
)

// fakeRuntime is a minimal Runtime implementation for testing detection.
type fakeRuntime struct{ name string }

func (f *fakeRuntime) Connect(context.Context) error { return nil }
func (f *fakeRuntime) IsConnected() bool             { return true }
func (f *fakeRuntime) SetDisconnected()              {}
func (f *fakeRuntime) Close() error                  { return nil }
func (f *fakeRuntime) Name() string                  { return f.name }
func (f *fakeRuntime) DiscoverAll(context.Context) ([]*container.Container, error) {
	return nil, nil
}
func (f *fakeRuntime) StreamEvents(context.Context) <-chan RuntimeEvent {
	return make(chan RuntimeEvent)
}
func (f *fakeRuntime) StatsSnapshot(context.Context, string) (*RawStats, error) {
	return nil, nil
}
func (f *fakeRuntime) FetchLogs(context.Context, string, int, bool) ([]string, error) {
	return nil, nil
}
func (f *fakeRuntime) StreamLogs(context.Context, string, int, bool) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeRuntime) GetHealthInfo(context.Context, string) (*HealthInfo, error) {
	return nil, nil
}

func resetFactories() {
	factoryMu.Lock()
	factories = map[string]Factory{}
	factoryMu.Unlock()
}

func registerFake(name string) {
	Register(name, func(ctx context.Context, logger *slog.Logger) (Runtime, error) {
		return &fakeRuntime{name: name}, nil
	})
}

func TestDetect_EnvOverrideDocker(t *testing.T) {
	resetFactories()
	registerFake("docker")
	t.Setenv("PULSEBOARD_RUNTIME", "docker")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	rt, err := Detect(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Name() != "docker" {
		t.Fatalf("expected docker, got %s", rt.Name())
	}
}

func TestDetect_EnvOverrideKubernetes(t *testing.T) {
	resetFactories()
	registerFake("kubernetes")
	t.Setenv("PULSEBOARD_RUNTIME", "kubernetes")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	rt, err := Detect(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Name() != "kubernetes" {
		t.Fatalf("expected kubernetes, got %s", rt.Name())
	}
}

func TestDetect_InvalidOverride(t *testing.T) {
	resetFactories()
	registerFake("docker")
	t.Setenv("PULSEBOARD_RUNTIME", "invalid")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	_, err := Detect(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error for invalid PULSEBOARD_RUNTIME")
	}
}

func TestDetect_KubernetesServiceHost(t *testing.T) {
	resetFactories()
	registerFake("docker")
	registerFake("kubernetes")
	t.Setenv("PULSEBOARD_RUNTIME", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBECONFIG", "")

	rt, err := Detect(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Name() != "kubernetes" {
		t.Fatalf("expected kubernetes, got %s", rt.Name())
	}
}

func TestDetect_KubernetesServiceHostNoFactory(t *testing.T) {
	resetFactories()
	registerFake("docker")
	t.Setenv("PULSEBOARD_RUNTIME", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBECONFIG", "")

	_, err := Detect(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error when K8s detected but no factory")
	}
}

func TestDetect_DockerFallback(t *testing.T) {
	resetFactories()
	registerFake("docker")
	t.Setenv("PULSEBOARD_RUNTIME", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	// Use a temp dir as HOME so no default kubeconfig is found
	t.Setenv("HOME", t.TempDir())

	rt, err := Detect(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Name() != "docker" {
		t.Fatalf("expected docker, got %s", rt.Name())
	}
}

func TestDetect_NoRuntime(t *testing.T) {
	resetFactories()
	t.Setenv("PULSEBOARD_RUNTIME", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())

	_, err := Detect(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error when no runtime registered")
	}
}

func TestDetect_KubeconfigEnvFallback(t *testing.T) {
	resetFactories()
	registerFake("docker")
	registerFake("kubernetes")
	t.Setenv("PULSEBOARD_RUNTIME", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	// Create a fake kubeconfig file
	tmpDir := t.TempDir()
	kubeconfig := tmpDir + "/config"
	if err := os.WriteFile(kubeconfig, []byte("apiVersion: v1"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)

	rt, err := Detect(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Name() != "kubernetes" {
		t.Fatalf("expected kubernetes via KUBECONFIG, got %s", rt.Name())
	}
}
