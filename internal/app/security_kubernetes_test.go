// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeExposures []kubernetes.ServiceExposure

func (f fakeExposures) ListServiceExposures(context.Context) ([]kubernetes.ServiceExposure, error) {
	return f, nil
}

type fakeWorkloads []*container.Container

func (f fakeWorkloads) ListContainers(_ context.Context, opts container.ListContainersOpts) ([]*container.Container, error) {
	if !opts.IncludeIgnored || opts.AgentFilter == nil || *opts.AgentFilter != uid.LocalAgent {
		return nil, nil
	}
	return f, nil
}

type securityAlert struct {
	containerID string
	recover     bool
}

func TestScanKubernetesSecurity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var alerts []securityAlert
	secSvc := security.NewService(security.Deps{
		Logger: logger,
		AlertCallback: func(id string, _ string, _ []security.Insight, isRecover bool) {
			alerts = append(alerts, securityAlert{id, isRecover})
		},
	})

	workloads := fakeWorkloads{
		{ID: "w-web", ExternalID: "shop/Deployment/web", Name: "web", RuntimeType: "kubernetes"},
		{ID: "w-db", ExternalID: "shop/StatefulSet/db", Name: "db", RuntimeType: "kubernetes", IsIgnored: true},
		{ID: "w-local", ExternalID: "abc", Name: "sidecar", RuntimeType: "docker"},
	}
	exposed := fakeExposures{
		{WorkloadID: "shop/Deployment/web", Service: "shop/web-lb", ServiceType: "LoadBalancer", Port: 443, TargetPort: 8443, Protocol: "tcp"},
		{WorkloadID: "shop/StatefulSet/db", Service: "shop/db-np", ServiceType: "NodePort", Port: 5432, TargetPort: 5432, NodePort: 31432, Protocol: "tcp"},
	}

	ScanKubernetesSecurity(context.Background(), exposed, workloads, secSvc, logger)

	web := secSvc.GetContainerInsights("w-web")
	require.Equal(t, 1, web.Count)
	assert.Equal(t, security.ServiceLoadBalancer, web.Insights[0].Type)
	assert.Zero(t, secSvc.GetContainerInsights("w-db").Count, "an ignored workload raises nothing")
	assert.Equal(t, []securityAlert{{"w-web", false}}, alerts, "the dangerous_configuration circuit is fed")

	ScanKubernetesSecurity(context.Background(), fakeExposures{}, workloads, secSvc, logger)

	assert.Zero(t, secSvc.GetContainerInsights("w-web").Count)
	assert.Equal(t, securityAlert{"w-web", true}, alerts[len(alerts)-1], "a Service removed resolves its alert")
}
