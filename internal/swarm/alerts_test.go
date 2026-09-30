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
