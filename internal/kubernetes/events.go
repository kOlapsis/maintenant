package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"time"

	cmodel "github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/runtime"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const informerResync = 30 * time.Second

// streamEvents turns pod and controller changes into runtime events, from informers of its own, until
// ctx ends, the runtime closes or the API server stops answering; the channel then closes.
func (r *Runtime) streamEvents(ctx context.Context) <-chan runtime.RuntimeEvent {
	out := make(chan runtime.RuntimeEvent, 128)

	clientset, err := r.client()
	if err != nil {
		r.logger.Error("kubernetes event stream not started", "error", err)
		close(out)
		return out
	}
	factory := informers.NewSharedInformerFactory(clientset, informerResync)

	podInformer := factory.Core().V1().Pods().Informer()
	depInformer := factory.Apps().V1().Deployments().Informer()
	ssInformer := factory.Apps().V1().StatefulSets().Informer()
	dsInformer := factory.Apps().V1().DaemonSets().Informer()

	emit := func(evt runtime.RuntimeEvent) {
		select {
		case out <- evt:
		case <-ctx.Done():
		default:
			r.logger.Warn("K8s event channel full, dropping event", "action", evt.Action, "id", evt.ExternalID)
		}
	}

	// Pod events — primary source for start/stop/die/health.
	_, _ = podInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			pod, ok := obj.(*corev1.Pod)
			return ok && r.nsFilter.IsAllowed(pod.Namespace)
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				if hasControllerOwner(pod) {
					return // controller-level events handled separately
				}
				emit(podToEvent("start", pod))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldPod := oldObj.(*corev1.Pod)
				newPod := newObj.(*corev1.Pod)
				if hasControllerOwner(newPod) {
					return
				}
				events := podUpdateEvents(oldPod, newPod)
				for _, e := range events {
					emit(e)
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod, ok := deletedObject[*corev1.Pod](obj)
				if !ok || hasControllerOwner(pod) {
					return
				}
				emit(podToEvent("destroy", pod))
			},
		},
	})

	watchController(r, depInformer, emit, deploymentToEvent)
	watchController(r, ssInformer, emit, statefulSetToEvent)
	watchController(r, dsInformer, emit, daemonSetToEvent)

	stop := make(chan struct{})
	factory.Start(stop)
	go func() {
		r.waitForLoss(ctx, clientset)
		close(stop)
		// Shutdown returns once no handler can run, so nothing sends on the closed channel.
		factory.Shutdown()
		close(out)
	}()

	return out
}

// watchController emits the start, update and destroy events of one controller kind in the allowed namespaces.
func watchController[T metav1.Object](r *Runtime, informer cache.SharedIndexInformer, emit func(runtime.RuntimeEvent), toEvent func(string, T) runtime.RuntimeEvent) {
	_, _ = informer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			o, ok := obj.(T)
			return ok && r.nsFilter.IsAllowed(o.GetNamespace())
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				emit(toEvent("start", obj.(T)))
			},
			UpdateFunc: func(_, newObj interface{}) {
				emit(toEvent("update", newObj.(T)))
			},
			DeleteFunc: func(obj interface{}) {
				if o, ok := deletedObject[T](obj); ok {
					emit(toEvent("destroy", o))
				}
			},
		},
	})
}

// deletedObject unwraps the object of a delete notification, tombstone included.
func deletedObject[T any](obj interface{}) (T, bool) {
	if o, ok := obj.(T); ok {
		return o, true
	}
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		o, ok := tombstone.Obj.(T)
		return o, ok
	}
	var zero T
	return zero, false
}

// waitForLoss returns when ctx ends, the runtime closes, or probeMisses probes in a row find the API server unreachable.
func (r *Runtime) waitForLoss(ctx context.Context, clientset k8s.Interface) {
	ticker := time.NewTicker(r.probeEvery)
	defer ticker.Stop()
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
		}
		err := probeAPIServer(ctx, clientset, r.probeEvery)
		if err == nil {
			misses = 0
			continue
		}
		if ctx.Err() != nil {
			return
		}
		misses++
		r.logger.Warn("kubernetes API server unreachable", "error", err, "misses", misses, "of", r.probeMisses)
		if misses >= r.probeMisses {
			return
		}
	}
}

// probeAPIServer fails only when the API server cannot be reached: any answer, a refusal included, proves it is up.
func probeAPIServer(ctx context.Context, clientset k8s.Interface, timeout time.Duration) error {
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := clientset.CoreV1().Namespaces().List(pctx, metav1.ListOptions{Limit: 1})
	var status k8serrors.APIStatus
	if err != nil && !errors.As(err, &status) {
		return err
	}
	return nil
}

func podToEvent(action string, pod *corev1.Pod) runtime.RuntimeEvent {
	state, errorDetail := podState(pod)
	healthStatus := ""
	if state == cmodel.StateRunning {
		healthStatus = "healthy"
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				healthStatus = "unhealthy"
				break
			}
		}
	}

	return runtime.RuntimeEvent{
		Action:       action,
		ExternalID:   fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		Name:         pod.Name,
		HealthStatus: healthStatus,
		ErrorDetail:  errorDetail,
		Timestamp:    time.Now(),
		Labels:       pod.Labels,
	}
}

func podUpdateEvents(oldPod, newPod *corev1.Pod) []runtime.RuntimeEvent {
	var events []runtime.RuntimeEvent

	oldState, _ := podState(oldPod)
	newState, newDetail := podState(newPod)

	if oldState != newState {
		action := "update"
		switch newState {
		case cmodel.StateRunning:
			action = "start"
		case cmodel.StateExited, cmodel.StateDead:
			action = "die"
		case cmodel.StateCompleted:
			action = "stop"
		}

		events = append(events, runtime.RuntimeEvent{
			Action:      action,
			ExternalID:  fmt.Sprintf("%s/%s", newPod.Namespace, newPod.Name),
			Name:        newPod.Name,
			ErrorDetail: newDetail,
			Timestamp:   time.Now(),
			Labels:      newPod.Labels,
		})
	}

	// Check health change.
	oldHealth := podHealthString(oldPod)
	newHealth := podHealthString(newPod)
	if oldHealth != newHealth && newHealth != "" {
		events = append(events, runtime.RuntimeEvent{
			Action:       "health_status",
			ExternalID:   fmt.Sprintf("%s/%s", newPod.Namespace, newPod.Name),
			Name:         newPod.Name,
			HealthStatus: newHealth,
			Timestamp:    time.Now(),
			Labels:       newPod.Labels,
		})
	}

	return events
}

func podHealthString(pod *corev1.Pod) string {
	if pod.Status.Phase != corev1.PodRunning {
		return ""
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if !cs.Ready {
			return "unhealthy"
		}
	}
	if len(pod.Status.ContainerStatuses) > 0 {
		return "healthy"
	}
	return ""
}

func deploymentToEvent(action string, dep *appsv1.Deployment) runtime.RuntimeEvent {
	_, errorDetail := deploymentState(dep)
	evt := controllerEvent(action, "Deployment", dep.ObjectMeta, dep.Status.ReadyReplicas, desiredReplicas(dep.Spec.Replicas))
	evt.ErrorDetail = errorDetail
	return evt
}

func statefulSetToEvent(action string, ss *appsv1.StatefulSet) runtime.RuntimeEvent {
	return controllerEvent(action, "StatefulSet", ss.ObjectMeta, ss.Status.ReadyReplicas, desiredReplicas(ss.Spec.Replicas))
}

func daemonSetToEvent(action string, ds *appsv1.DaemonSet) runtime.RuntimeEvent {
	return controllerEvent(action, "DaemonSet", ds.ObjectMeta, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
}

func controllerEvent(action, kind string, meta metav1.ObjectMeta, ready, desired int32) runtime.RuntimeEvent {
	health := ""
	switch {
	case desired > 0 && ready >= desired:
		health = "healthy"
	case ready > 0:
		health = "starting"
	case desired > 0:
		health = "unhealthy"
	}
	return runtime.RuntimeEvent{
		Action:       action,
		ExternalID:   workloadID(meta.Namespace, kind, meta.Name),
		Name:         meta.Name,
		HealthStatus: health,
		Timestamp:    time.Now(),
		Labels:       meta.Labels,
	}
}
