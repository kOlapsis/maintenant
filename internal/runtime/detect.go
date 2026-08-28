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
			return nil, fmt.Errorf("docker runtime unavailable: %w. Set MAINTENANT_RUNTIME or ensure Docker socket is mounted", err)
		}
		logger.Info("runtime initialized", "runtime", rt.Name(), "method", "auto_detect_docker")
		return rt, nil
	}

	return nil, fmt.Errorf("no runtime detected; ensure Docker socket is mounted or set MAINTENANT_RUNTIME; registered: %v", registeredNames())
}

func registeredNames() []string {
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
