// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

var ignoreLabels = map[string]string{"maintenant.ignore": "true"}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func underReplicated(id, name string, labels map[string]string) *SwarmService {
	return &SwarmService{ServiceID: id, Name: name, Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 1, Labels: labels}
}

type alertSink struct{ events []alert.Event }

func (s *alertSink) record(evt alert.Event) { s.events = append(s.events, evt) }

func (s *alertSink) take() []alert.Event {
	out := s.events
	s.events = nil
	return out
}

func TestReplicaHealthChecker_IgnoredServiceRaisesNothing(t *testing.T) {
	sink := &alertSink{}
	rhc := NewReplicaHealthChecker(quietLogger())
	rhc.SetAlertDelay(0)
	rhc.SetAlertCallback(sink.record)

	svc := underReplicated("svc1", "prod_web", ignoreLabels)
	rhc.Check([]*SwarmService{svc})
	rhc.Check([]*SwarmService{svc})
	assert.Empty(t, sink.take())
}

func TestReplicaHealthChecker_ServiceIgnoredWhileAlertedResolves(t *testing.T) {
	sink := &alertSink{}
	rhc := NewReplicaHealthChecker(quietLogger())
	rhc.SetAlertDelay(0)
	rhc.SetAlertCallback(sink.record)

	rhc.Check([]*SwarmService{underReplicated("svc1", "prod_web", nil)})
	rhc.Check([]*SwarmService{underReplicated("svc1", "prod_web", nil)})
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "svc1", fired[0].EntityID)

	rhc.Check([]*SwarmService{underReplicated("svc1", "prod_web", ignoreLabels)})
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "svc1", recovered[0].EntityID)
}

func TestReplicaHealthChecker_EachServiceHasItsOwnAlert(t *testing.T) {
	sink := &alertSink{}
	rhc := NewReplicaHealthChecker(quietLogger())
	rhc.SetAlertDelay(0)
	rhc.SetAlertCallback(sink.record)

	services := []*SwarmService{underReplicated("svc1", "prod_web", nil), underReplicated("svc2", "prod_api", nil)}
	rhc.Check(services)
	rhc.Check(services)
	fired := sink.take()
	require.Len(t, fired, 2)
	assert.ElementsMatch(t, []string{"svc1", "svc2"}, []string{fired[0].EntityID, fired[1].EntityID})
}

func TestEventProcessor_IgnoredServiceRaisesNothing(t *testing.T) {
	sink := &alertSink{}
	ep := NewEventProcessor(nil, quietLogger())
	ep.SetAlertCallback(sink.record)

	ep.checkReplicaHealth(underReplicated("svc1", "prod_web", ignoreLabels))
	assert.Empty(t, sink.take())

	ep.checkReplicaHealth(underReplicated("svc1", "prod_web", nil))
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "svc1", fired[0].EntityID)

	ep.checkReplicaHealth(underReplicated("svc1", "prod_web", ignoreLabels))
	recovered := sink.take()
	require.Len(t, recovered, 1, "ignoring a degraded service resolves its alert")
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "svc1", recovered[0].EntityID)
}

type updateClient struct {
	ServiceClient
	svc swarm.Service
}

func (c updateClient) ServiceInspect(context.Context, string) (swarm.Service, error) {
	return c.svc, nil
}
func (c updateClient) TaskList(context.Context) ([]swarm.Task, error) { return nil, nil }

func rollingUpdate(state swarm.UpdateState, labels map[string]string) swarm.Service {
	return swarm.Service{
		ID:           "svc1",
		Spec:         swarm.ServiceSpec{Annotations: swarm.Annotations{Name: "prod_web", Labels: labels}},
		UpdateStatus: &swarm.UpdateStatus{State: state, Message: "update paused due to failure"},
	}
}

func TestUpdateTracker_IgnoredServiceRaisesNothing(t *testing.T) {
	for _, state := range []swarm.UpdateState{swarm.UpdateStatePaused, swarm.UpdateStateRollbackCompleted} {
		t.Run(string(state), func(t *testing.T) {
			sink := &alertSink{}
			ignored := NewUpdateTracker(updateClient{svc: rollingUpdate(state, ignoreLabels)}, quietLogger())
			ignored.SetAlertCallback(sink.record)
			ignored.CheckService(context.Background(), "svc1")
			assert.Empty(t, sink.take())

			watched := NewUpdateTracker(updateClient{svc: rollingUpdate(state, nil)}, quietLogger())
			watched.SetAlertCallback(sink.record)
			watched.CheckService(context.Background(), "svc1")
			fired := sink.take()
			require.Len(t, fired, 1)
			assert.Equal(t, "svc1", fired[0].EntityID)
		})
	}
}

func TestCrashLoopDetector_RecoveryNamesTheService(t *testing.T) {
	sink := &alertSink{}
	cld := NewCrashLoopDetector(quietLogger())
	cld.SetAlertCallback(sink.record)

	for range crashLoopThreshold {
		cld.RecordFailure("svc1", "prod_web", "exit 1")
	}
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "svc1", fired[0].EntityID)

	cld.services["svc1"].lastFailure = time.Now().Add(-crashLoopRecoveryTime)
	cld.CheckRecoveries()
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "svc1", recovered[0].EntityID)
	assert.Equal(t, "prod_web", recovered[0].EntityName)
}

func TestNodeService_AlertsCarryTheNodeID(t *testing.T) {
	sink := &alertSink{}
	ns := NewNodeService(nil, nil, quietLogger())
	ns.SetAlertCallback(sink.record)

	ready := func(id, host string) *SwarmNode {
		return &SwarmNode{NodeID: id, Hostname: host, Role: "worker", Status: "ready", Availability: "active"}
	}
	down := func(id, host string) *SwarmNode {
		n := ready(id, host)
		n.Status = "down"
		return n
	}

	ns.detectTransitions(ready("n1", "worker-1"), down("n1", "worker-1"))
	ns.detectTransitions(ready("n2", "worker-2"), down("n2", "worker-2"))
	fired := sink.take()
	require.Len(t, fired, 2)
	assert.ElementsMatch(t, []string{"n1", "n2"}, []string{fired[0].EntityID, fired[1].EntityID})

	drained := ready("n1", "worker-1")
	drained.Availability = "drain"
	ns.detectTransitions(ready("n1", "worker-1"), drained)
	ns.detectTransitions(down("n2", "worker-2"), ready("n2", "worker-2"))
	events := sink.take()
	require.Len(t, events, 2)
	assert.Equal(t, "node_drain", events[0].AlertType)
	assert.Equal(t, "n1", events[0].EntityID)
	assert.True(t, events[1].IsRecover)
	assert.Equal(t, "n2", events[1].EntityID)
}

func TestUpdateTracker_CompletedUpdateResolvesItsAlerts(t *testing.T) {
	sink := &alertSink{}
	client := &updateClient{svc: rollingUpdate(swarm.UpdateStatePaused, nil)}
	ut := NewUpdateTracker(client, quietLogger())
	ut.SetAlertCallback(sink.record)

	ut.CheckService(context.Background(), "svc1")
	client.svc = rollingUpdate(swarm.UpdateStateRollbackCompleted, nil)
	ut.CheckService(context.Background(), "svc1")
	require.Len(t, sink.take(), 2)

	client.svc = rollingUpdate(swarm.UpdateStateCompleted, nil)
	ut.CheckService(context.Background(), "svc1")
	recovered := sink.take()
	require.Len(t, recovered, 2)
	var types []string
	for _, evt := range recovered {
		assert.True(t, evt.IsRecover)
		assert.Equal(t, "svc1", evt.EntityID)
		types = append(types, evt.AlertType)
	}
	assert.ElementsMatch(t, []string{"update_stalled", "update_rollback"}, types)

	ut.CheckService(context.Background(), "svc1")
	assert.Empty(t, sink.take(), "a resolved alert is not resolved twice")
}

func TestUpdateTracker_IgnoringAServiceResolvesItsAlerts(t *testing.T) {
	sink := &alertSink{}
	client := &updateClient{svc: rollingUpdate(swarm.UpdateStateRollbackCompleted, nil)}
	ut := NewUpdateTracker(client, quietLogger())
	ut.SetAlertCallback(sink.record)

	ut.CheckService(context.Background(), "svc1")
	require.Len(t, sink.take(), 1)

	client.svc = rollingUpdate(swarm.UpdateStateRollbackCompleted, ignoreLabels)
	ut.CheckService(context.Background(), "svc1")
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
}

func TestResume_TakesOverAlertsLeftByThePreviousRun(t *testing.T) {
	active := []*alert.Alert{
		{Source: "swarm", AlertType: "replica_unhealthy", EntityType: "swarm_service", EntityID: "svc1", EntityName: "prod_web"},
		{Source: "swarm", AlertType: "crash_loop", EntityType: "swarm_service", EntityID: "svc2", EntityName: "prod_api"},
		{Source: "swarm", AlertType: "update_rollback", EntityType: "swarm_service", EntityID: "svc3", EntityName: "prod_db"},
		{Source: "kubernetes", AlertType: "crash_loop", EntityType: "pod", EntityID: "shop/api"},
	}

	t.Run("replicas", func(t *testing.T) {
		sink := &alertSink{}
		rhc := NewReplicaHealthChecker(quietLogger())
		rhc.SetAlertCallback(sink.record)
		rhc.Resume(active)
		rhc.Check([]*SwarmService{{ServiceID: "svc1", Name: "prod_web", Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 3}})
		recovered := sink.take()
		require.Len(t, recovered, 1)
		assert.True(t, recovered[0].IsRecover)
		assert.Equal(t, "svc1", recovered[0].EntityID)
	})

	t.Run("crash loop", func(t *testing.T) {
		sink := &alertSink{}
		cld := NewCrashLoopDetector(quietLogger())
		cld.SetAlertCallback(sink.record)
		cld.Resume(active)
		require.True(t, cld.IsCrashLooping("svc2"))
		cld.services["svc2"].lastFailure = time.Now().Add(-crashLoopRecoveryTime)
		cld.CheckRecoveries()
		recovered := sink.take()
		require.Len(t, recovered, 1)
		assert.Equal(t, "svc2", recovered[0].EntityID)
		assert.Equal(t, "prod_api", recovered[0].EntityName)
	})

	t.Run("updates", func(t *testing.T) {
		sink := &alertSink{}
		ut := NewUpdateTracker(&updateClient{svc: rollingUpdate(swarm.UpdateStateCompleted, nil)}, quietLogger())
		ut.SetAlertCallback(sink.record)
		ut.Resume(active)
		ut.CheckService(context.Background(), "svc3")
		recovered := sink.take()
		require.Len(t, recovered, 1)
		assert.Equal(t, "update_rollback", recovered[0].AlertType)
		assert.Equal(t, "svc3", recovered[0].EntityID)
	})
}
