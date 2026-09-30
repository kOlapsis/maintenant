// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
)

// An event without an entity name reaches every channel titled "Alert: ".
func TestAlertEvents_BuiltInThisPackageNameTheirEntity(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	built := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isAlertEventType(lit.Type) {
				return true
			}
			built++
			assert.True(t, setsField(lit, "EntityName"), "%s builds an alert.Event without EntityName", fset.Position(lit.Pos()))
			return true
		})
	}
	assert.NotZero(t, built)
}

func isAlertEventType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "alert" && sel.Sel.Name == "Event"
}

func setsField(lit *ast.CompositeLit, field string) bool {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
				return true
			}
		}
	}
	return false
}

func TestContainerHealthAlerts_NameTheContainer(t *testing.T) {
	a, _ := newTestApp(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a.alertEngine.Start(ctx)

	now := time.Now()
	healthy := container.HealthHealthy
	_, err := a.containerStore.InsertContainer(ctx, &container.Container{
		ExternalID:        "ext-web",
		Name:              "web",
		Image:             "nginx:1.27",
		State:             container.StateRunning,
		HealthStatus:      &healthy,
		HasHealthCheck:    true,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	})
	require.NoError(t, err)

	healthAlerts := func() []*alert.Alert {
		all, err := a.alertStore.ListAlerts(ctx, alert.ListAlertsOpts{Source: alert.SourceContainer})
		require.NoError(t, err)
		var out []*alert.Alert
		for _, al := range all {
			if al.AlertType == "health_unhealthy" {
				out = append(out, al)
			}
		}
		return out
	}

	a.containerSvc.ProcessEvent(ctx, container.ContainerEvent{
		Action: "health_status", ExternalID: "ext-web", HealthStatus: string(container.HealthUnhealthy), Timestamp: now,
	})
	require.Eventually(t, func() bool { return len(healthAlerts()) == 1 }, 2*time.Second, 10*time.Millisecond)

	a.containerSvc.ProcessEvent(ctx, container.ContainerEvent{
		Action: "health_status", ExternalID: "ext-web", HealthStatus: string(container.HealthHealthy), Timestamp: now.Add(time.Second),
	})
	require.Eventually(t, func() bool { return len(healthAlerts()) == 2 }, 2*time.Second, 10*time.Millisecond,
		"the recovery is stored next to the alert it resolves")

	for _, al := range healthAlerts() {
		assert.Equal(t, "web", al.EntityName, "%s alert", al.Status)
	}
}
