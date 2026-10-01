// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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

// Detect auto-detects the container runtime or uses the MAINTENANT_RUNTIME env override.
// Detection order: env override → KUBERNETES_SERVICE_HOST → KUBECONFIG → Docker socket.
func Detect(ctx context.Context, logger *slog.Logger) (Runtime, error) {
	return DetectWithOverride(ctx, logger, "")
}

// DetectWithOverride is like Detect but allows the caller to pass an explicit
// override (e.g. from a CLI flag). When override is empty, the MAINTENANT_RUNTIME
// env variable is consulted as a fallback.
func DetectWithOverride(ctx context.Context, logger *slog.Logger, override string) (Runtime, error) {
	if override == "" {
		override = os.Getenv("MAINTENANT_RUNTIME")
	}

	if override != "" {
		f, ok := factories[override]
		if !ok {
			return nil, fmt.Errorf("unknown MAINTENANT_RUNTIME=%q; registered runtimes: %v", override, registeredNames())
		}
		logger.Info("runtime selected via override", "runtime", override)
		rt, err := f(ctx, logger)
		if err != nil {
			return nil, fmt.Errorf("runtime %q from MAINTENANT_RUNTIME failed: %w", override, err)
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
				return nil, fmt.Errorf("kubernetes in-cluster runtime failed: %w", err)
			}
			logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_in_cluster")
			return rt, nil
		}
		return nil, fmt.Errorf("kubernetes environment detected (KUBERNETES_SERVICE_HOST set) but kubernetes runtime not yet implemented; registered: %v", registeredNames())
	}

	// Try KUBECONFIG for out-of-cluster K8s development.
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		if rt := fromKubeconfig(ctx, logger, kubeconfig, "auto_detect_kubeconfig"); rt != nil {
			return rt, nil
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		defaultKubeconfig := home + "/.kube/config"
		if _, err := os.Stat(defaultKubeconfig); err == nil {
			if rt := fromKubeconfig(ctx, logger, defaultKubeconfig, "auto_detect_default_kubeconfig"); rt != nil {
				return rt, nil
			}
		}
	}

	// Try Docker.
	if f, ok := factories["docker"]; ok {
		rt, err := f(ctx, logger)
		if err != nil {
			return nil, fmt.Errorf("docker runtime unavailable: %w. Set MAINTENANT_RUNTIME or ensure Docker socket is mounted", err)
		}
		logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_docker")
		return rt, nil
	}

	return nil, fmt.Errorf("no runtime detected; ensure Docker socket is mounted or set MAINTENANT_RUNTIME; registered: %v", registeredNames())
}

// fromKubeconfig returns the Kubernetes runtime when the cluster of kubeconfig answers, nil to let detection move on to Docker.
func fromKubeconfig(ctx context.Context, logger *slog.Logger, kubeconfig, method string) Runtime {
	f, ok := factories["kubernetes"]
	if !ok {
		return nil
	}
	logger.Info("detected Kubernetes via kubeconfig", "kubeconfig", kubeconfig, "method", method)
	rt, err := f(ctx, logger)
	if err != nil {
		logger.Warn("kubeconfig present but Kubernetes runtime failed, falling back to Docker", "kubeconfig", kubeconfig, "error", err)
		return nil
	}
	if err := rt.TryConnect(ctx); err != nil {
		_ = rt.Close()
		logger.Warn("kubeconfig present but its cluster is unreachable, falling back to Docker; set MAINTENANT_RUNTIME=kubernetes to wait for that cluster instead",
			"kubeconfig", kubeconfig, "error", err)
		return nil
	}
	logger.Info("runtime initialized", "runtime", rt.Name(), "method", method)
	return rt
}

func registeredNames() []string {
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
