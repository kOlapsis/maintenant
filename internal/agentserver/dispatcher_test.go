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

package agentserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// --- mock handlers ---

type mockContainerHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.ContainerEvent
	calledWithMeta    EventMeta
	returnErr         error
}

func (m *mockContainerHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.ContainerEvent, meta EventMeta) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	m.calledWithMeta = meta
	return m.returnErr
}

type mockInventoryHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.ContainerInventory
	calledWithMeta    EventMeta
	returnErr         error
}

func (m *mockInventoryHandler) HandleAgentInventory(_ context.Context, agentID string, ev *agentpb.ContainerInventory, meta EventMeta) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	m.calledWithMeta = meta
	return m.returnErr
}

type mockEndpointHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.EndpointEvent
	calledWithMeta    EventMeta
	returnErr         error
}

func (m *mockEndpointHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.EndpointEvent, meta EventMeta) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	m.calledWithMeta = meta
	return m.returnErr
}

type mockHeartbeatHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.HeartbeatEvent
	returnErr         error
}

func (m *mockHeartbeatHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.HeartbeatEvent) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	return m.returnErr
}

type mockResourceHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.ResourceSample
	calledWithMeta    EventMeta
	returnErr         error
}

func (m *mockResourceHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.ResourceSample, meta EventMeta) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	m.calledWithMeta = meta
	return m.returnErr
}

type mockCertificateHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.CertificateInfo
	returnErr         error
}

func (m *mockCertificateHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.CertificateInfo) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	return m.returnErr
}

// --- tests ---

const dispatchAgentID = "agt-dispatch-001"

func TestDispatcher_ContainerEventRoutedToContainerHandler(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	ev := &agentpb.ContainerEvent{ContainerId: "ctr-abc"}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Container{Container: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
}

func TestDispatcher_InventoryRoutedToInventoryHandler(t *testing.T) {
	h := &mockInventoryHandler{}
	var synced []string
	d := NewDispatcher(DispatchDeps{
		Inventory: h,
		LabelSync: func(_ context.Context, _, _, externalID string, _ map[string]string) {
			synced = append(synced, externalID)
		},
	})

	ev := &agentpb.ContainerInventory{Containers: []*agentpb.ContainerEvent{
		{ContainerId: "ctr-1"}, {ContainerId: "ctr-2"},
	}}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Inventory{Inventory: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
	assert.Equal(t, []string{"ctr-1", "ctr-2"}, synced,
		"label discovery must run for every container in the snapshot")
}

func TestDispatcher_EndpointEventRoutedToEndpointHandler(t *testing.T) {
	h := &mockEndpointHandler{}
	d := NewDispatcher(DispatchDeps{Endpoint: h})

	ev := &agentpb.EndpointEvent{}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Endpoint{Endpoint: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
}

func TestDispatcher_HeartbeatEventRoutedToHeartbeatHandler(t *testing.T) {
	h := &mockHeartbeatHandler{}
	d := NewDispatcher(DispatchDeps{Heartbeat: h})

	ev := &agentpb.HeartbeatEvent{}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Heartbeat{Heartbeat: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
}

func TestDispatcher_ResourceEventRoutedToResourceHandler(t *testing.T) {
	h := &mockResourceHandler{}
	d := NewDispatcher(DispatchDeps{Resource: h})

	ev := &agentpb.ResourceSample{}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Resource{Resource: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
}

func TestDispatcher_CertificateEventRoutedToCertificateHandler(t *testing.T) {
	h := &mockCertificateHandler{}
	d := NewDispatcher(DispatchDeps{Certificate: h})

	ev := &agentpb.CertificateInfo{}
	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Certificate{Certificate: ev},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
	assert.Same(t, ev, h.calledWithEvent)
}

func TestDispatcher_NilContainerHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Container: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilEndpointHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Endpoint: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilHeartbeatHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Heartbeat: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Heartbeat{Heartbeat: &agentpb.HeartbeatEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilResourceHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Resource: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilCertificateHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Certificate: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Certificate{Certificate: &agentpb.CertificateInfo{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	assert.NoError(t, err)
}

func TestDispatcher_HandlerErrorIsWrappedAndReturned(t *testing.T) {
	handlerErr := errors.New("downstream failure")
	h := &mockContainerHandler{returnErr: handlerErr}
	d := NewDispatcher(DispatchDeps{Container: h})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.Error(t, err)
	assert.ErrorIs(t, err, handlerErr)
}

func TestDispatcher_EndpointHandlerErrorIsWrappedAndReturned(t *testing.T) {
	handlerErr := errors.New("endpoint write failed")
	h := &mockEndpointHandler{returnErr: handlerErr}
	d := NewDispatcher(DispatchDeps{Endpoint: h})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.Error(t, err)
	assert.ErrorIs(t, err, handlerErr)
}

func TestDispatcher_RejectsSpoofedAgentID(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	// Event claims to belong to another agent than the authenticated one.
	evt := &agentpb.AgentEvent{
		AgentId: "agt-victim-999",
		Body:    &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.Error(t, err)
	assert.Empty(t, h.calledWithAgentID, "a spoofed event must never reach the handler")
}

func TestDispatcher_UsesAuthenticatedIDNotWireValue(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	// Empty wire agent_id: the authenticated identity must be used regardless.
	evt := &agentpb.AgentEvent{
		Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{}},
	}

	err := d.Dispatch(context.Background(), dispatchAgentID, evt)

	require.NoError(t, err)
	assert.Equal(t, dispatchAgentID, h.calledWithAgentID)
}

func TestDispatcher_AllNilHandlersNoError(t *testing.T) {
	d := NewDispatcher(DispatchDeps{})

	// Any event body type with all-nil handlers must not panic or error.
	for _, evt := range []*agentpb.AgentEvent{
		{AgentId: dispatchAgentID, Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{}}},
		{AgentId: dispatchAgentID, Body: &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{}}},
		{AgentId: dispatchAgentID, Body: &agentpb.AgentEvent_Heartbeat{Heartbeat: &agentpb.HeartbeatEvent{}}},
		{AgentId: dispatchAgentID, Body: &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{}}},
		{AgentId: dispatchAgentID, Body: &agentpb.AgentEvent_Certificate{Certificate: &agentpb.CertificateInfo{}}},
	} {
		err := d.Dispatch(context.Background(), dispatchAgentID, evt)
		assert.NoError(t, err)
	}
}

func TestDispatcher_DestroyedEventSyncsLabelsWithNilLabels(t *testing.T) {
	var gotLabels map[string]string
	var called bool
	d := NewDispatcher(DispatchDeps{
		LabelSync: func(_ context.Context, _, _, _ string, labels map[string]string) {
			called = true
			gotLabels = labels
		},
	})

	err := d.Dispatch(context.Background(), dispatchAgentID, &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{
			ContainerId: "ctr-1",
			Name:        "demo",
			Labels:      map[string]string{"maintenant.endpoint.web": "http://x/health"},
			Destroyed:   true,
		}},
	})

	require.NoError(t, err)
	require.True(t, called)
	assert.Nil(t, gotLabels, "a destroyed container must retract its label-discovered monitors")
}

// --- observation time validation (FR-016) ---

func TestDispatcher_FutureObservedAtBeyondSkewIsRejected(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	err := d.Dispatch(context.Background(), dispatchAgentID, &agentpb.AgentEvent{
		AgentId:    dispatchAgentID,
		ObservedAt: timestamppb.New(time.Now().Add(5 * time.Minute)),
		Body:       &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{ContainerId: "ctr-future"}},
	})

	require.Error(t, err)
	assert.Nil(t, h.calledWithEvent, "a rejected event must never reach its handler")
	assert.Equal(t, uint64(1), d.RejectedEvents(dispatchAgentID))
}

func TestDispatcher_ObservedAtOlderThanRetentionIsRejected(t *testing.T) {
	h := &mockResourceHandler{}
	d := NewDispatcher(DispatchDeps{Resource: h})

	err := d.Dispatch(context.Background(), dispatchAgentID, &agentpb.AgentEvent{
		AgentId:    dispatchAgentID,
		ObservedAt: timestamppb.New(time.Now().Add(-25 * time.Hour)),
		Body:       &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{ContainerId: "ctr-old"}},
	})

	require.Error(t, err)
	assert.Nil(t, h.calledWithEvent, "a rejected event must never reach its handler")
	assert.Equal(t, uint64(1), d.RejectedEvents(dispatchAgentID))
}

func TestDispatcher_RejectionsAreCountedPerAgent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Container: &mockContainerHandler{}})

	evt := func() *agentpb.AgentEvent {
		return &agentpb.AgentEvent{
			ObservedAt: timestamppb.New(time.Now().Add(-48 * time.Hour)),
			Body:       &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{ContainerId: "ctr"}},
		}
	}
	require.Error(t, d.Dispatch(context.Background(), "agent-a", evt()))
	require.Error(t, d.Dispatch(context.Background(), "agent-a", evt()))
	require.Error(t, d.Dispatch(context.Background(), "agent-b", evt()))

	assert.Equal(t, uint64(2), d.RejectedEvents("agent-a"))
	assert.Equal(t, uint64(1), d.RejectedEvents("agent-b"))
	assert.Equal(t, uint64(0), d.RejectedEvents("agent-c"))
}

func TestDispatcher_ObservedAtWithinToleranceIsPassedThroughUnclamped(t *testing.T) {
	h := &mockResourceHandler{}
	d := NewDispatcher(DispatchDeps{Resource: h})

	observed := time.Now().Add(-3 * time.Hour).Truncate(time.Millisecond)
	err := d.Dispatch(context.Background(), dispatchAgentID, &agentpb.AgentEvent{
		AgentId:    dispatchAgentID,
		Replayed:   true,
		ObservedAt: timestamppb.New(observed),
		Body:       &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{ContainerId: "ctr"}},
	})

	require.NoError(t, err)
	assert.True(t, observed.Equal(h.calledWithMeta.ObservedAt), "the observation time must reach the handler untouched")
	assert.True(t, h.calledWithMeta.Replayed)
	assert.Equal(t, uint64(0), d.RejectedEvents(dispatchAgentID))
}

func TestDispatcher_MissingObservedAtFallsBackToReceiveTime(t *testing.T) {
	h := &mockEndpointHandler{}
	d := NewDispatcher(DispatchDeps{Endpoint: h})

	before := time.Now()
	err := d.Dispatch(context.Background(), dispatchAgentID, &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{Url: "https://example.test"}},
	})

	require.NoError(t, err)
	assert.False(t, h.calledWithMeta.ObservedAt.Before(before), "an agent without observed_at must be dated on reception")
	assert.False(t, h.calledWithMeta.Replayed)
	assert.Equal(t, uint64(0), d.RejectedEvents(dispatchAgentID))
}
