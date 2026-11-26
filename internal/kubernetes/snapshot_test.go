// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeSnapshotSource is a static SnapshotSource for exercising
// SnapshotFromRuntime without a live cluster.
type fakeSnapshotSource struct {
	namespaces []string
	pods       []K8sPod
	nodes      []K8sNode
	events     []K8sEventRef
	nodesErr   error
	eventsErr  error
}

func (f fakeSnapshotSource) ListNamespaces(context.Context) ([]string, error) {
	return f.namespaces, nil
}
func (f fakeSnapshotSource) ListWorkloads(context.Context, []string) ([]K8sWorkloadGroup, error) {
	return nil, nil
}
func (f fakeSnapshotSource) ListPods(context.Context, []string, PodFilters) ([]K8sPod, error) {
	return f.pods, nil
}
func (f fakeSnapshotSource) ListNodes(context.Context) ([]K8sNode, error) {
	return f.nodes, f.nodesErr
}
func (f fakeSnapshotSource) ListAllEvents(context.Context) ([]K8sEventRef, error) {
	return f.events, f.eventsErr
}

// TestSnapshotFromRuntime_TalliesRunningPodsPerNode verifies that the snapshot
// derives each node's RunningPods from the pods scheduled on it, since a Node
// object carries no running-pod count.
func TestSnapshotFromRuntime_TalliesRunningPodsPerNode(t *testing.T) {
	src := fakeSnapshotSource{
		namespaces: []string{"demo"},
		pods: []K8sPod{
			{Name: "a", NodeName: "node-1"},
			{Name: "b", NodeName: "node-1"},
			{Name: "c", NodeName: "node-2"},
			{Name: "unscheduled", NodeName: ""}, // not yet placed: counted for no node
		},
		nodes: []K8sNode{
			{Name: "node-1"},
			{Name: "node-2"},
			{Name: "node-3"}, // no pods scheduled
		},
	}

	snap, err := SnapshotFromRuntime(context.Background(), src)
	if err != nil {
		t.Fatalf("SnapshotFromRuntime: %v", err)
	}

	want := map[string]int{"node-1": 2, "node-2": 1, "node-3": 0}
	if len(snap.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(snap.Nodes), len(want))
	}
	for _, n := range snap.Nodes {
		if n.RunningPods != want[n.Name] {
			t.Errorf("node %q: RunningPods = %d, want %d", n.Name, n.RunningPods, want[n.Name])
		}
	}
}

func TestListAllEvents_KeepsTheMostRecent(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	objs := make([]kruntime.Object, 0, 250)
	for i := range 250 {
		objs = append(objs, &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("e%03d", i), Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "web"},
			Reason:         fmt.Sprintf("r%03d", i),
			LastTimestamp:  metav1.NewTime(base.Add(time.Duration(i) * time.Second)),
		})
	}
	r := newStreamRuntime(t, fake.NewClientset(objs...))

	events, err := r.ListAllEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAllEvents: %v", err)
	}
	if len(events) != 100 {
		t.Fatalf("got %d events, want 100", len(events))
	}
	if events[0].Reason != "r249" || events[len(events)-1].Reason != "r150" {
		t.Fatalf("kept %s..%s, want r249..r150", events[0].Reason, events[len(events)-1].Reason)
	}
}

func TestListAllEvents_KeepsWarningsFirst(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	objs := make([]kruntime.Object, 0, 310)
	for i := range 310 {
		typ := corev1.EventTypeNormal
		if i < 10 {
			typ = corev1.EventTypeWarning
		}
		objs = append(objs, &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("e%03d", i), Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "web"},
			Type:           typ,
			Reason:         fmt.Sprintf("r%03d", i),
			LastTimestamp:  metav1.NewTime(base.Add(time.Duration(i) * time.Second)),
		})
	}
	r := newStreamRuntime(t, fake.NewClientset(objs...))

	events, err := r.ListAllEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAllEvents: %v", err)
	}
	if len(events) != 100 {
		t.Fatalf("got %d events, want 100", len(events))
	}
	if events[0].Reason != "r009" || events[9].Reason != "r000" {
		t.Fatalf("warnings are %s..%s, want r009..r000", events[0].Reason, events[9].Reason)
	}
	if events[10].Reason != "r309" || events[99].Reason != "r220" {
		t.Fatalf("others are %s..%s, want r309..r220", events[10].Reason, events[99].Reason)
	}
}

func TestMaxSnapshotEvents_StaysUnderAQuarterOfTheMessageLimit(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	largest := &agentpb.K8SEventMsg{
		Type:              corev1.EventTypeWarning,
		Reason:            long(128),
		Message:           long(1024),
		Source:            long(63 + 1 + 253),
		FirstSeen:         timestamppb.Now(),
		LastSeen:          timestamppb.Now(),
		Count:             1 << 30,
		InvolvedKind:      long(63),
		InvolvedNamespace: long(63),
		InvolvedName:      long(253),
	}
	if total := maxSnapshotEvents * proto.Size(largest); total > 1<<20/4 {
		t.Fatalf("%d events of %d bytes weigh %d bytes, over a quarter of 1 MiB", maxSnapshotEvents, proto.Size(largest), total)
	}
}

func TestListAllEvents_DatesEventsCarryingOnlyAnEventTime(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	objs := make([]kruntime.Object, 0, 250)
	for i := range 250 {
		at := base.Add(time.Duration(i) * time.Second)
		e := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("e%03d", i), Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "web"},
			Reason:         fmt.Sprintf("r%03d", i),
		}
		if i >= 190 {
			e.EventTime = metav1.NewMicroTime(at)
		} else {
			e.FirstTimestamp = metav1.NewTime(at)
			e.LastTimestamp = metav1.NewTime(at)
		}
		objs = append(objs, e)
	}
	r := newStreamRuntime(t, fake.NewClientset(objs...))

	events, err := r.ListAllEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAllEvents: %v", err)
	}
	if len(events) != 100 {
		t.Fatalf("got %d events, want 100", len(events))
	}
	for i := range 60 {
		want := fmt.Sprintf("r%03d", 249-i)
		if events[i].Reason != want {
			t.Fatalf("event %d is %s, want %s", i, events[i].Reason, want)
		}
		if events[i].LastSeen.IsZero() || events[i].FirstSeen.IsZero() {
			t.Fatalf("event %s has an empty date", want)
		}
	}
}

func TestEventLastSeen_Order(t *testing.T) {
	at := func(s int64) time.Time { return time.Unix(s, 0) }
	e := &corev1.Event{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(at(1))}}
	if got := eventLastSeen(e); !got.Equal(at(1)) {
		t.Fatalf("creation: got %v", got)
	}
	e.EventTime = metav1.NewMicroTime(at(2))
	if got := eventLastSeen(e); !got.Equal(at(2)) {
		t.Fatalf("event time: got %v", got)
	}
	if got := eventFirstSeen(e); !got.Equal(at(2)) {
		t.Fatalf("first seen: got %v", got)
	}
	e.Series = &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(at(3))}
	if got := eventLastSeen(e); !got.Equal(at(3)) {
		t.Fatalf("series: got %v", got)
	}
	e.LastTimestamp = metav1.NewTime(at(4))
	if got := eventLastSeen(e); !got.Equal(at(4)) {
		t.Fatalf("last timestamp: got %v", got)
	}
}

func TestSnapshotFromRuntime_ListErrors(t *testing.T) {
	forbidden := func(resource string) error {
		return k8serrors.NewForbidden(schema.GroupResource{Resource: resource}, "", errors.New("denied"))
	}
	throttled := k8serrors.NewTooManyRequests("slow down", 1)

	tests := []struct {
		name      string
		nodesErr  error
		eventsErr error
		wantErr   bool
	}{
		{name: "nodes forbidden", nodesErr: forbidden("nodes")},
		{name: "events forbidden", eventsErr: forbidden("events")},
		{name: "nodes throttled", nodesErr: throttled, wantErr: true},
		{name: "events throttled", eventsErr: throttled},
		{name: "events timeout", eventsErr: context.DeadlineExceeded},
		{name: "nodes timeout", nodesErr: context.DeadlineExceeded, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fakeSnapshotSource{
				namespaces: []string{"demo"},
				pods:       []K8sPod{{Name: "a"}},
				nodesErr:   tt.nodesErr,
				eventsErr:  tt.eventsErr,
			}
			snap, err := SnapshotFromRuntime(context.Background(), src)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(snap.Pods) != 1 {
				t.Fatalf("got %d pods, want 1", len(snap.Pods))
			}
		})
	}
}
