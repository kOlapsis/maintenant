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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// --- mock handlers ---

type mockContainerHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.ContainerEvent
	returnErr         error
}

func (m *mockContainerHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.ContainerEvent) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	return m.returnErr
}

type mockInventoryHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.ContainerInventory
	returnErr         error
}

func (m *mockInventoryHandler) HandleAgentInventory(_ context.Context, agentID string, ev *agentpb.ContainerInventory) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
	return m.returnErr
}

type mockEndpointHandler struct {
	calledWithAgentID string
	calledWithEvent   *agentpb.EndpointEvent
	returnErr         error
}

func (m *mockEndpointHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.EndpointEvent) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
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
	returnErr         error
}

func (m *mockResourceHandler) HandleAgentEvent(_ context.Context, agentID string, ev *agentpb.ResourceSample) error {
	m.calledWithAgentID = agentID
	m.calledWithEvent = ev
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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilEndpointHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Endpoint: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Endpoint{Endpoint: &agentpb.EndpointEvent{}},
	}

	err := d.Dispatch(context.Background(), evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilHeartbeatHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Heartbeat: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Heartbeat{Heartbeat: &agentpb.HeartbeatEvent{}},
	}

	err := d.Dispatch(context.Background(), evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilResourceHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Resource: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{}},
	}

	err := d.Dispatch(context.Background(), evt)

	assert.NoError(t, err)
}

func TestDispatcher_NilCertificateHandlerSilentlyIgnoresEvent(t *testing.T) {
	d := NewDispatcher(DispatchDeps{Certificate: nil})

	evt := &agentpb.AgentEvent{
		AgentId: dispatchAgentID,
		Body:    &agentpb.AgentEvent_Certificate{Certificate: &agentpb.CertificateInfo{}},
	}

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

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

	err := d.Dispatch(context.Background(), evt)

	require.Error(t, err)
	assert.ErrorIs(t, err, handlerErr)
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
		err := d.Dispatch(context.Background(), evt)
		assert.NoError(t, err)
	}
}
