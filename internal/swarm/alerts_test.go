// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
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

type replicaClient struct {
	ServiceClient
	desired uint64
	running int
	labels  map[string]string
}

func (c *replicaClient) service() swarm.Service {
	return swarm.Service{
		ID: "svc1",
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{Name: "prod_web", Labels: c.labels},
			Mode:        swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &c.desired}},
		},
	}
}

func (c *replicaClient) ServiceInspect(context.Context, string) (swarm.Service, error) {
	return c.service(), nil
}

func (c *replicaClient) ServiceList(context.Context) ([]swarm.Service, error) {
	return []swarm.Service{c.service()}, nil
}

func (c *replicaClient) TaskList(context.Context) ([]swarm.Task, error) {
	tasks := make([]swarm.Task, c.running)
	for i := range tasks {
		tasks[i] = swarm.Task{ServiceID: "svc1", Status: swarm.TaskStatus{State: swarm.TaskStateRunning}}
	}
	return tasks, nil
}

func serviceUpdated() runtime.RuntimeEvent {
	return runtime.RuntimeEvent{ResourceType: runtime.ResourceService, Action: "update", ExternalID: "svc1"}
}

func replicaSetup(client *replicaClient, delay time.Duration) (*EventProcessor, *ServiceDiscovery, *ReplicaHealthChecker, *alertSink) {
	sink := &alertSink{}
	rhc := NewReplicaHealthChecker(quietLogger())
	rhc.SetAlertDelay(delay)
	rhc.SetAlertCallback(sink.record)
	disc := NewServiceDiscovery(client, quietLogger())
	ep := NewEventProcessor(disc, quietLogger())
	ep.SetReplicaChecker(rhc)
	return ep, disc, rhc, sink
}

func TestEventProcessor_ScaleUpWaitsForTheReplicaDelay(t *testing.T) {
	ctx := context.Background()
	client := &replicaClient{desired: 4, running: 2}
	ep, disc, rhc, sink := replicaSetup(client, defaultReplicaAlertDelay)

	ep.ProcessEvent(ctx, serviceUpdated())
	assert.Empty(t, sink.take(), "a service being scaled up is not under-replicated for long yet")

	client.running = 4
	_, err := disc.DiscoverAll(ctx)
	require.NoError(t, err)
	rhc.Check(disc.ListServices())
	assert.Empty(t, sink.take(), "the new tasks came up within the delay: nothing to raise nor resolve")
}

func TestEventProcessor_ResolvesTheReplicaAlertOfTheChecker(t *testing.T) {
	ctx := context.Background()
	client := &replicaClient{desired: 3, running: 1}
	ep, disc, rhc, sink := replicaSetup(client, 0)

	ep.ProcessEvent(ctx, serviceUpdated())
	rhc.Check(disc.ListServices())
	fired := sink.take()
	require.Len(t, fired, 1, "the event and the check share one under-replication clock")
	assert.False(t, fired[0].IsRecover)

	client.running = 3
	ep.ProcessEvent(ctx, serviceUpdated())
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "svc1", recovered[0].EntityID)

	rhc.Check(disc.ListServices())
	assert.Empty(t, sink.take(), "a resolved alert is not resolved twice")
}

func TestEventProcessor_IgnoredServiceRaisesNothing(t *testing.T) {
	ctx := context.Background()
	ep, _, _, sink := replicaSetup(&replicaClient{desired: 3, running: 1, labels: ignoreLabels}, 0)

	ep.ProcessEvent(ctx, serviceUpdated())
	ep.ProcessEvent(ctx, serviceUpdated())
	assert.Empty(t, sink.take())
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

func failedTask(id, node string, exitCode int, at time.Time) SwarmTask {
	task := SwarmTask{
		TaskID: id, ServiceID: "svc1", NodeID: node, State: "failed", DesiredState: "shutdown",
		Error: fmt.Sprintf("task: non-zero exit (%d)", exitCode), Timestamp: at,
	}
	if exitCode != 0 {
		task.ExitCode = &exitCode
	}
	return task
}

func tasksOf(labels map[string]string, tasks ...SwarmTask) TopologySnapshot {
	return TopologySnapshot{
		Services: []SwarmService{{ServiceID: "svc1", Name: "prod_web", Mode: "replicated", DesiredReplicas: 2, Labels: labels}},
		Tasks:    tasks,
	}
}

func TestCrashLoopDetector_CountsTheFailedTasksOfEveryNode(t *testing.T) {
	sink := &alertSink{}
	var failedEvents []string
	cld := NewCrashLoopDetector(quietLogger())
	cld.SetAlertCallback(sink.record)
	cld.SetEventCallback(func(eventType string, data interface{}) {
		if eventType == event.SwarmTaskFailed {
			failedEvents = append(failedEvents, data.(map[string]interface{})["task_id"].(string))
		}
	})

	now := time.Now()
	rolledOut := SwarmTask{TaskID: "old", ServiceID: "svc1", NodeID: "n1", State: "shutdown", DesiredState: "shutdown", Timestamp: now}
	code := 143
	rolledOut.ExitCode = &code
	history := []SwarmTask{
		rolledOut,
		failedTask("stopped", "n1", 143, now),
		failedTask("stale", "n2", 1, now.Add(-crashLoopWindow-time.Minute)),
		failedTask("crash", "n2", 1, now),
		failedTask("oom", "n3", 137, now),
	}

	cld.ObserveTasks(tasksOf(nil, history...))
	cld.ObserveTasks(tasksOf(nil, history...))
	assert.Empty(t, sink.take(), "a shutdown, a stop sent by hand and a failure out of the window are no crash; a task counts once")
	assert.Equal(t, []string{"crash", "oom"}, failedEvents)

	cld.ObserveTasks(tasksOf(nil, append(history, failedTask("again", "n1", 2, now))...))
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "crash_loop", fired[0].AlertType)
	assert.Equal(t, "svc1", fired[0].EntityID)
	assert.Equal(t, 3, fired[0].Details["failure_count"])
	assert.True(t, cld.IsCrashLooping("svc1"))
}

func TestCrashLoopDetector_IgnoredServiceRaisesNothing(t *testing.T) {
	sink := &alertSink{}
	cld := NewCrashLoopDetector(quietLogger())
	cld.SetAlertCallback(sink.record)

	now := time.Now()
	cld.ObserveTasks(tasksOf(ignoreLabels, failedTask("a", "n1", 1, now), failedTask("b", "n2", 1, now), failedTask("c", "n3", 1, now)))
	assert.Empty(t, sink.take())
	assert.False(t, cld.IsCrashLooping("svc1"))
}

func TestCrashLoopDetector_RecoveryNamesTheService(t *testing.T) {
	sink := &alertSink{}
	cld := NewCrashLoopDetector(quietLogger())
	cld.SetAlertCallback(sink.record)

	now := time.Now()
	cld.ObserveTasks(tasksOf(nil, failedTask("a", "n1", 1, now), failedTask("b", "n1", 1, now), failedTask("c", "n1", 1, now)))
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

func TestNodeService_QuorumAlertResolvesWhenTheQuorumIsBack(t *testing.T) {
	sink := &alertSink{}
	ns := NewNodeService(nil, nil, quietLogger())
	ns.SetAlertCallback(sink.record)

	ns.checkQuorum(3, 1)
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "quorum_degraded", fired[0].AlertType)
	assert.Equal(t, alert.SeverityCritical, fired[0].Severity)
	assert.False(t, fired[0].IsRecover)

	ns.checkQuorum(3, 1)
	assert.Empty(t, sink.take(), "a degraded quorum alerts once")

	ns.checkQuorum(3, 2)
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "quorum_degraded", recovered[0].AlertType)

	ns.checkQuorum(3, 3)
	assert.Empty(t, sink.take())
}

func TestNodeService_ResumesAQuorumAlert(t *testing.T) {
	sink := &alertSink{}
	ns := NewNodeService(nil, nil, quietLogger())
	ns.SetAlertCallback(sink.record)
	ns.Resume([]*alert.Alert{{Source: "swarm", AlertType: "quorum_degraded", EntityType: "swarm_cluster"}})

	ns.checkQuorum(3, 3)
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
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
