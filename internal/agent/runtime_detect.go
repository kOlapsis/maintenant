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

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"

	dockerclient "github.com/docker/docker/client"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Runtime identifies the container orchestration environment detected on this host.
type Runtime string

const (
	RuntimeDocker     Runtime = "docker"
	RuntimeSwarm      Runtime = "swarm"
	RuntimeKubernetes Runtime = "kubernetes"
)

// ErrAmbiguousRuntime is returned when more than one runtime is detected.
type ErrAmbiguousRuntime struct {
	Detected []Runtime
}

func (e *ErrAmbiguousRuntime) Error() string {
	return fmt.Sprintf("ambiguous runtime: detected %v — use --runtime to override", e.Detected)
}

// ErrNoRuntimeDetected is returned when no container runtime is reachable.
var ErrNoRuntimeDetected = errors.New("no container runtime detected (tried docker and kubernetes)")

// Detect auto-detects the container runtime present on the host.
// If override is non-empty and a valid Runtime value, it is returned immediately
// without any probe (the caller is responsible for ensuring the runtime is usable).
// Detection order: Swarm (active node) > Docker > Kubernetes.
// Returns ErrAmbiguousRuntime if both Docker/Swarm and Kubernetes are detected.
func Detect(ctx context.Context, override string) (Runtime, error) {
	if override != "" {
		switch Runtime(override) {
		case RuntimeDocker, RuntimeSwarm, RuntimeKubernetes:
			return Runtime(override), nil
		default:
			return "", fmt.Errorf("unknown runtime override %q (valid: docker, swarm, kubernetes)", override)
		}
	}

	var detected []Runtime

	// Docker / Swarm probe
	dockerRuntime, dockerErr := probeDocker(ctx)
	if dockerErr == nil {
		detected = append(detected, dockerRuntime)
	}

	// Kubernetes probe
	if probeKubernetes() {
		detected = append(detected, RuntimeKubernetes)
	}

	switch len(detected) {
	case 0:
		return "", ErrNoRuntimeDetected
	case 1:
		return detected[0], nil
	default:
		return "", &ErrAmbiguousRuntime{Detected: detected}
	}
}

func probeDocker(ctx context.Context) (Runtime, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return "", err
	}
	defer cli.Close()

	info, err := cli.Info(ctx)
	if err != nil {
		return "", err
	}

	if info.Swarm.LocalNodeState == "active" {
		return RuntimeSwarm, nil
	}
	return RuntimeDocker, nil
}

func probeKubernetes() bool {
	// In-cluster check
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		if _, err := rest.InClusterConfig(); err == nil {
			return true
		}
	}

	// KUBECONFIG or default ~/.kube/config
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = home + "/.kube/config"
		}
	}
	if kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); err == nil {
			_, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			return err == nil
		}
	}
	return false
}
