// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func serviceRuntime(nsFilter *NamespaceFilter, objects ...k8sruntime.Object) *Runtime {
	return &Runtime{
		logger:    slog.Default(),
		nsFilter:  nsFilter,
		clientset: fake.NewClientset(objects...),
		prevCPU:   make(map[string]*cpuPrev),
		stopCh:    make(chan struct{}),
	}
}

func service(ns, name string, typ corev1.ServiceType, selector map[string]string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       corev1.ServiceSpec{Type: typ, Selector: selector, Ports: ports},
	}
}

func TestListServiceExposures(t *testing.T) {
	web := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web", "tier": "front"}},
		}},
	}
	db := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "db"},
		Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
		}},
	}
	debug := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "debug", Labels: map[string]string{"app": "debug"}}}
	otherNS := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "staging", Name: "web"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
		}},
	}

	rt := serviceRuntime(NewNamespaceFilter("", ""),
		web, db, debug, otherNS,
		service("shop", "web-lb", corev1.ServiceTypeLoadBalancer, map[string]string{"app": "web"},
			corev1.ServicePort{Port: 443, TargetPort: intstr.FromInt32(8443), Protocol: corev1.ProtocolTCP}),
		service("shop", "db-np", corev1.ServiceTypeNodePort, map[string]string{"app": "db"},
			corev1.ServicePort{Port: 5432, TargetPort: intstr.FromString("pg"), NodePort: 31432}),
		service("shop", "debug-np", corev1.ServiceTypeNodePort, map[string]string{"app": "debug"},
			corev1.ServicePort{Port: 8080, NodePort: 30080, Protocol: corev1.ProtocolUDP}),
		service("shop", "web-internal", corev1.ServiceTypeClusterIP, map[string]string{"app": "web"},
			corev1.ServicePort{Port: 80}),
		service("shop", "no-selector", corev1.ServiceTypeLoadBalancer, nil,
			corev1.ServicePort{Port: 80}),
	)

	exposures, err := rt.ListServiceExposures(context.Background())
	require.NoError(t, err)

	byWorkload := map[string]ServiceExposure{}
	for _, e := range exposures {
		byWorkload[e.WorkloadID] = e
	}
	require.Len(t, byWorkload, 3, "the ClusterIP service, the selector-less one and the other namespace expose nothing")

	assert.Equal(t, ServiceExposure{
		WorkloadID: "shop/Deployment/web", Service: "shop/web-lb", ServiceType: "LoadBalancer",
		Port: 443, TargetPort: 8443, Protocol: "tcp",
	}, byWorkload["shop/Deployment/web"])
	assert.Equal(t, ServiceExposure{
		WorkloadID: "shop/StatefulSet/db", Service: "shop/db-np", ServiceType: "NodePort",
		Port: 5432, NodePort: 31432, Protocol: "tcp",
	}, byWorkload["shop/StatefulSet/db"])
	assert.Equal(t, "udp", byWorkload["shop/debug"].Protocol)
}

func TestListServiceExposures_NamespaceFilter(t *testing.T) {
	web := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
		}},
	}
	rt := serviceRuntime(NewNamespaceFilter("", "shop"), web,
		service("shop", "web-lb", corev1.ServiceTypeLoadBalancer, map[string]string{"app": "web"},
			corev1.ServicePort{Port: 443}))

	exposures, err := rt.ListServiceExposures(context.Background())
	require.NoError(t, err)
	assert.Empty(t, exposures)
}
