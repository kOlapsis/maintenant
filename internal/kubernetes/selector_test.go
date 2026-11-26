// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetWorkload_SelectorWithMatchExpressionsKeepsItsOwnPods(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"api"}},
			}},
		},
	}
	pod := func(name, app string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop", Labels: map[string]string{"app": app}}}
	}
	rt := withClient(&Runtime{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		nsFilter: NewNamespaceFilter("", ""),
		prevCPU:  make(map[string]*cpuPrev),
		stopCh:   make(chan struct{}),
	}, fake.NewClientset(dep, pod("api-1", "api"), pod("db-1", "db")))

	_, pods, _, err := rt.GetWorkload(context.Background(), "shop/Deployment/api")
	require.NoError(t, err)
	require.Len(t, pods, 1)
	assert.Equal(t, "api-1", pods[0].Name)

	sel, err := rt.controllerSelector(context.Background(), "shop", "Deployment", "api")
	require.NoError(t, err)
	assert.Equal(t, "app in (api)", sel)
}
