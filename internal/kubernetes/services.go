// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceExposure is one port a LoadBalancer or NodePort Service opens outside
// the cluster, attributed to a workload its selector matches.
type ServiceExposure struct {
	WorkloadID  string // the workload's external id, as discoverAll mints it
	Service     string // namespace/name
	ServiceType string
	Port        int
	TargetPort  int // 0 when the Service targets a named port
	NodePort    int
	Protocol    string
}

type selectableWorkload struct {
	id        string
	namespace string
	labels    map[string]string
}

// ListServiceExposures returns the ports LoadBalancer and NodePort Services
// expose, one entry per workload each Service selects.
func (r *Runtime) ListServiceExposures(ctx context.Context) ([]ServiceExposure, error) {
	list, err := r.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	var exposing []corev1.Service
	for _, svc := range list.Items {
		if !r.nsFilter.IsAllowed(svc.Namespace) || len(svc.Spec.Selector) == 0 {
			continue
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
			exposing = append(exposing, svc)
		}
	}
	if len(exposing) == 0 {
		return nil, nil
	}

	workloads, err := r.selectableWorkloads(ctx)
	if err != nil {
		return nil, err
	}

	var out []ServiceExposure
	for _, svc := range exposing {
		selector := labels.SelectorFromSet(svc.Spec.Selector)
		for _, w := range workloads {
			if w.namespace != svc.Namespace || !selector.Matches(labels.Set(w.labels)) {
				continue
			}
			for _, p := range svc.Spec.Ports {
				out = append(out, ServiceExposure{
					WorkloadID:  w.id,
					Service:     svc.Namespace + "/" + svc.Name,
					ServiceType: string(svc.Spec.Type),
					Port:        int(p.Port),
					TargetPort:  numericTargetPort(p.TargetPort),
					NodePort:    int(p.NodePort),
					Protocol:    servicePortProtocol(p.Protocol),
				})
			}
		}
	}
	return out, nil
}

// selectableWorkloads lists what a Service selector can match: the pod
// templates of the controllers and the labels of bare pods. A kind the RBAC
// denies is left out, as in discoverAll.
func (r *Runtime) selectableWorkloads(ctx context.Context) ([]selectableWorkload, error) {
	var out []selectableWorkload
	keep := func(ns string) bool { return r.nsFilter.IsAllowed(ns) }
	skip := func(kind string, err error) error {
		if k8serrors.IsForbidden(err) {
			r.logger.Warn("RBAC: forbidden to list "+kind+", its exposure is not analysed", "error", err)
			return nil
		}
		return fmt.Errorf("list %s: %w", kind, err)
	}

	deployments, err := r.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if err := skip("deployments", err); err != nil {
			return nil, err
		}
	} else {
		for _, d := range deployments.Items {
			if keep(d.Namespace) {
				out = append(out, selectableWorkload{fmt.Sprintf("%s/Deployment/%s", d.Namespace, d.Name), d.Namespace, d.Spec.Template.Labels})
			}
		}
	}

	statefulSets, err := r.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if err := skip("statefulsets", err); err != nil {
			return nil, err
		}
	} else {
		for _, s := range statefulSets.Items {
			if keep(s.Namespace) {
				out = append(out, selectableWorkload{fmt.Sprintf("%s/StatefulSet/%s", s.Namespace, s.Name), s.Namespace, s.Spec.Template.Labels})
			}
		}
	}

	daemonSets, err := r.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if err := skip("daemonsets", err); err != nil {
			return nil, err
		}
	} else {
		for _, d := range daemonSets.Items {
			if keep(d.Namespace) {
				out = append(out, selectableWorkload{fmt.Sprintf("%s/DaemonSet/%s", d.Namespace, d.Name), d.Namespace, d.Spec.Template.Labels})
			}
		}
	}

	pods, err := r.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if err := skip("pods", err); err != nil {
			return nil, err
		}
	} else {
		for i := range pods.Items {
			p := &pods.Items[i]
			if keep(p.Namespace) && !hasControllerOwner(p) {
				out = append(out, selectableWorkload{fmt.Sprintf("%s/%s", p.Namespace, p.Name), p.Namespace, p.Labels})
			}
		}
	}
	return out, nil
}

func numericTargetPort(p intstr.IntOrString) int {
	if p.Type == intstr.Int {
		return int(p.IntVal)
	}
	return 0
}

func servicePortProtocol(p corev1.Protocol) string {
	if p == "" {
		return "tcp"
	}
	return strings.ToLower(string(p))
}
