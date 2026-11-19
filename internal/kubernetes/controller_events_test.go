package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kolapsis/maintenant/internal/runtime"
)

func TestStreamEvents_ControllersAddedByTheInformersEmitNoStart(t *testing.T) {
	zero := int32(0)
	three := int32(3)
	cs := fake.NewClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}, Spec: appsv1.DeploymentSpec{Replicas: &three}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop"}, Spec: appsv1.StatefulSetSpec{Replicas: &three}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "shop"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "shop"}, Spec: appsv1.DeploymentSpec{Replicas: &zero}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "shop"}},
	)
	r := newStreamRuntime(t, cs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := r.StreamEvents(ctx)

	var seen []runtime.RuntimeEvent
	synced := false
	deadline := time.After(5 * time.Second)
	for !synced {
		select {
		case evt := <-events:
			seen = append(seen, evt)
			synced = evt.ExternalID == "shop/bare"
		case <-deadline:
			t.Fatal("no event reached the stream: the informers are not running")
		}
	}
	settle := time.After(300 * time.Millisecond)
	for done := false; !done; {
		select {
		case evt := <-events:
			seen = append(seen, evt)
		case <-settle:
			done = true
		}
	}

	for _, evt := range seen {
		if evt.ExternalID != "shop/bare" {
			require.NotEqual(t, "start", evt.Action, "a controller present at startup must not be forced to running: %s", evt.ExternalID)
		}
	}
}
