package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
)

// Factory creates a Runtime from configuration and logger.
type Factory func(ctx context.Context, logger *slog.Logger) (Runtime, error)

var (
	factoryMu sync.Mutex
	factories = map[string]Factory{}
)

// Register adds a named runtime factory. Called from init() in runtime packages.
func Register(name string, f Factory) {
	factoryMu.Lock()
	factories[name] = f
	factoryMu.Unlock()
}

// Detect auto-detects the container runtime or uses the PULSEBOARD_RUNTIME override.
// Detection order: env override → KUBERNETES_SERVICE_HOST → KUBECONFIG → Docker socket.
func Detect(ctx context.Context, logger *slog.Logger) (Runtime, error) {
	override := os.Getenv("PULSEBOARD_RUNTIME")

	if override != "" {
		f, ok := factories[override]
		if !ok {
			return nil, fmt.Errorf("unknown PULSEBOARD_RUNTIME=%q; registered runtimes: %v", override, registeredNames())
		}
		logger.Info("runtime selected via override", "runtime", override)
		rt, err := f(ctx, logger)
		if err != nil {
			return nil, fmt.Errorf("runtime %q from PULSEBOARD_RUNTIME failed: %w", override, err)
		}
		logger.Info("runtime initialized", "runtime", rt.Name(), "method", "env_override")
		return rt, nil
	}

	// Auto-detect: Kubernetes first (in-cluster or KUBECONFIG), then Docker.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		if f, ok := factories["kubernetes"]; ok {
			logger.Info("detected Kubernetes in-cluster environment", "method", "KUBERNETES_SERVICE_HOST")
			rt, err := f(ctx, logger)
			if err != nil {
				return nil, fmt.Errorf("Kubernetes in-cluster runtime failed: %w", err)
			}
			logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_in_cluster")
			return rt, nil
		}
		return nil, fmt.Errorf("Kubernetes environment detected (KUBERNETES_SERVICE_HOST set) but kubernetes runtime not yet implemented; registered: %v", registeredNames())
	}

	// Try KUBECONFIG for out-of-cluster K8s development.
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		if f, ok := factories["kubernetes"]; ok {
			logger.Info("detected Kubernetes via KUBECONFIG", "kubeconfig", kubeconfig, "method", "KUBECONFIG")
			rt, err := f(ctx, logger)
			if err != nil {
				logger.Warn("KUBECONFIG present but Kubernetes runtime failed, falling back to Docker", "error", err)
			} else {
				logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_kubeconfig")
				return rt, nil
			}
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		defaultKubeconfig := home + "/.kube/config"
		if _, err := os.Stat(defaultKubeconfig); err == nil {
			if f, ok := factories["kubernetes"]; ok {
				logger.Info("detected default kubeconfig", "path", defaultKubeconfig, "method", "default_kubeconfig")
				rt, err := f(ctx, logger)
				if err != nil {
					logger.Warn("default kubeconfig present but Kubernetes runtime failed, falling back to Docker", "error", err)
				} else {
					logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_default_kubeconfig")
					return rt, nil
				}
			}
		}
	}

	// Try Docker.
	if f, ok := factories["docker"]; ok {
		rt, err := f(ctx, logger)
		if err != nil {
			return nil, fmt.Errorf("Docker runtime unavailable: %w. Set PULSEBOARD_RUNTIME or ensure Docker socket is mounted", err)
		}
		logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_docker")
		return rt, nil
	}

	return nil, fmt.Errorf("no runtime detected; ensure Docker socket is mounted or set PULSEBOARD_RUNTIME; registered: %v", registeredNames())
}

func registeredNames() []string {
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
