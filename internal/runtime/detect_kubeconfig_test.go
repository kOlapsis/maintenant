// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

type unreachableRuntime struct {
	fakeRuntime
	closed bool
}

func (u *unreachableRuntime) TryConnect(context.Context) error {
	return errors.New("dial tcp 10.0.0.1:6443: connect: connection refused")
}

func (u *unreachableRuntime) Close() error {
	u.closed = true
	return nil
}

func registerUnreachableKubernetes() *unreachableRuntime {
	rt := &unreachableRuntime{fakeRuntime: fakeRuntime{name: "kubernetes"}}
	Register("kubernetes", func(context.Context, *slog.Logger) (Runtime, error) { return rt, nil })
	return rt
}

func writeKubeconfig(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("apiVersion: v1"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetect_UnreachableKubeconfigClusterFallsBackToDocker(t *testing.T) {
	cases := map[string]func(t *testing.T){
		"KUBECONFIG": func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			writeKubeconfig(t, path)
			t.Setenv("KUBECONFIG", path)
		},
		"default kubeconfig": func(t *testing.T) {
			home := t.TempDir()
			writeKubeconfig(t, filepath.Join(home, ".kube", "config"))
			t.Setenv("HOME", home)
			t.Setenv("KUBECONFIG", "")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			resetFactories()
			registerFake("docker")
			kube := registerUnreachableKubernetes()
			t.Setenv("MAINTENANT_RUNTIME", "")
			t.Setenv("KUBERNETES_SERVICE_HOST", "")
			setup(t)

			rt, err := Detect(context.Background(), slog.Default())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rt.Name() != "docker" {
				t.Fatalf("expected the docker fallback, got %s", rt.Name())
			}
			if !kube.closed {
				t.Fatal("the abandoned kubernetes runtime was not closed")
			}
		})
	}
}

func TestDetect_UnreachableClusterKeptWhenNotGuessedFromAKubeconfig(t *testing.T) {
	cases := map[string]map[string]string{
		"MAINTENANT_RUNTIME forced": {"MAINTENANT_RUNTIME": "kubernetes", "KUBERNETES_SERVICE_HOST": ""},
		"in-cluster":                {"MAINTENANT_RUNTIME": "", "KUBERNETES_SERVICE_HOST": "10.0.0.1"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			resetFactories()
			registerFake("docker")
			registerUnreachableKubernetes()
			for k, v := range env {
				t.Setenv(k, v)
			}

			rt, err := Detect(context.Background(), slog.Default())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rt.Name() != "kubernetes" {
				t.Fatalf("expected kubernetes, got %s", rt.Name())
			}
		})
	}
}
