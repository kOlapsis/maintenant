// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/kolapsis/maintenant/internal/runtime"
)

const testProbeEvery = 20 * time.Millisecond

func streamRuntime(cs *fake.Clientset) *Runtime {
	return &Runtime{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		nsFilter:    NewNamespaceFilter("", ""),
		clientset:   cs,
		stopCh:      make(chan struct{}),
		probeEvery:  testProbeEvery,
		probeMisses: 2,
	}
}

func newStreamRuntime(t *testing.T, cs *fake.Clientset) *Runtime {
	t.Helper()
	r := streamRuntime(cs)
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// namespaceListFails makes the probe's namespace list fail with err while fail is set.
func namespaceListFails(cs *fake.Clientset, fail *atomic.Bool, err error) {
	cs.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, kruntime.Object, error) {
		if fail.Load() {
			return true, nil, err
		}
		return false, nil, nil
	})
}

func requireOpenFor(t *testing.T, events <-chan runtime.RuntimeEvent, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-events:
			require.True(t, ok, "the event stream closed while the API server answered")
		case <-deadline:
			return
		}
	}
}

func requireClosedWithin(t *testing.T, events <-chan runtime.RuntimeEvent, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("the event stream is still open after %s", d)
		}
	}
}

func TestStreamEvents_DeliversPodsCreatedWhileStreaming(t *testing.T) {
	cs := fake.NewClientset()
	r := newStreamRuntime(t, cs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := r.StreamEvents(ctx)

	// The fake API server does not replay what happened between the informer's
	// list and its watch, so keep creating pods until one comes through live.
	deadline := time.After(5 * time.Second)
	for i := 0; ; i++ {
		name := fmt.Sprintf("live-%d", i)
		_, err := cs.CoreV1().Pods("default").Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}, metav1.CreateOptions{})
		require.NoError(t, err)
		select {
		case evt := <-events:
			require.Equal(t, "start", evt.Action)
			require.True(t, strings.HasPrefix(evt.ExternalID, "default/live-"), evt.ExternalID)
			cancel()
			requireClosedWithin(t, events, time.Second)
			return
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("no event reached the stream: the informers are not running")
		}
	}
}

func TestStreamEvents_EndsWhenTheAPIServerIsLost(t *testing.T) {
	cs := fake.NewClientset()
	var down atomic.Bool
	namespaceListFails(cs, &down, errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"))
	r := newStreamRuntime(t, cs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := r.StreamEvents(ctx)
	requireOpenFor(t, events, 10*testProbeEvery)

	down.Store(true)
	requireClosedWithin(t, events, 2*time.Second)
}

func TestStreamEvents_ARefusalStillProvesTheAPIServerIsUp(t *testing.T) {
	cs := fake.NewClientset()
	var refusing atomic.Bool
	refusing.Store(true)
	namespaceListFails(cs, &refusing, k8serrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", errors.New("RBAC")))
	r := newStreamRuntime(t, cs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requireOpenFor(t, r.StreamEvents(ctx), 10*testProbeEvery)
}

func TestStreamEvents_EndsWhenTheRuntimeCloses(t *testing.T) {
	r := streamRuntime(fake.NewClientset())
	events := r.StreamEvents(context.Background())

	require.NoError(t, r.Close())

	requireClosedWithin(t, events, time.Second)
}
