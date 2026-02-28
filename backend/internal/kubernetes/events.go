package kubernetes

import (
	"context"
	"fmt"
	"time"

	cmodel "github.com/kolapsis/pulseboard/internal/container"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

// streamEvents uses SharedInformerFactory to watch pods and controllers,
// converting events to runtime.RuntimeEvent.
func (r *Runtime) streamEvents(ctx context.Context) <-chan pbruntime.RuntimeEvent {
	out := make(chan pbruntime.RuntimeEvent, 128)

	podInformer := r.factory.Core().V1().Pods().Informer()
	depInformer := r.factory.Apps().V1().Deployments().Informer()
	ssInformer := r.factory.Apps().V1().StatefulSets().Informer()
	dsInformer := r.factory.Apps().V1().DaemonSets().Informer()

	emit := func(evt pbruntime.RuntimeEvent) {
		select {
		case out <- evt:
		case <-ctx.Done():
		default:
			r.logger.Warn("K8s event channel full, dropping event", "action", evt.Action, "id", evt.ExternalID)
		}
	}

	// Pod events — primary source for start/stop/die/health.
	podInformer.AddEventHandler(cache.FilteringResourceEventHandler{
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
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if ok {
						pod, _ = tombstone.Obj.(*corev1.Pod)
					}
				}
				if pod == nil || hasControllerOwner(pod) {
					return
				}
				emit(podToEvent("destroy", pod))
			},
		},
	})

	// Deployment events — for controller-level rollout tracking.
	depInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			dep, ok := obj.(*appsv1.Deployment)
			return ok && r.nsFilter.IsAllowed(dep.Namespace)
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				dep := obj.(*appsv1.Deployment)
				emit(deploymentToEvent("start", dep))
			},
			UpdateFunc: func(_, newObj interface{}) {
				dep := newObj.(*appsv1.Deployment)
				emit(deploymentToEvent("update", dep))
			},
			DeleteFunc: func(obj interface{}) {
				dep, ok := obj.(*appsv1.Deployment)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if ok {
						dep, _ = tombstone.Obj.(*appsv1.Deployment)
					}
				}
				if dep != nil {
					emit(deploymentToEvent("destroy", dep))
				}
			},
		},
	})

	// StatefulSet events.
	ssInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			ss, ok := obj.(*appsv1.StatefulSet)
			return ok && r.nsFilter.IsAllowed(ss.Namespace)
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ss := obj.(*appsv1.StatefulSet)
				emit(statefulSetToEvent("start", ss))
			},
			UpdateFunc: func(_, newObj interface{}) {
				ss := newObj.(*appsv1.StatefulSet)
				emit(statefulSetToEvent("update", ss))
			},
			DeleteFunc: func(obj interface{}) {
				ss, ok := obj.(*appsv1.StatefulSet)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if ok {
						ss, _ = tombstone.Obj.(*appsv1.StatefulSet)
					}
				}
				if ss != nil {
					emit(statefulSetToEvent("destroy", ss))
				}
			},
		},
	})

	// DaemonSet events.
	dsInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			ds, ok := obj.(*appsv1.DaemonSet)
			return ok && r.nsFilter.IsAllowed(ds.Namespace)
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ds := obj.(*appsv1.DaemonSet)
				emit(daemonSetToEvent("start", ds))
			},
			UpdateFunc: func(_, newObj interface{}) {
				ds := newObj.(*appsv1.DaemonSet)
				emit(daemonSetToEvent("update", ds))
			},
			DeleteFunc: func(obj interface{}) {
				ds, ok := obj.(*appsv1.DaemonSet)
				if !ok {
					tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
					if ok {
						ds, _ = tombstone.Obj.(*appsv1.DaemonSet)
					}
				}
				if ds != nil {
					emit(daemonSetToEvent("destroy", ds))
				}
			},
		},
	})

	go func() {
		<-ctx.Done()
		close(out)
	}()

	return out
}

func podToEvent(action string, pod *corev1.Pod) pbruntime.RuntimeEvent {
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

	return pbruntime.RuntimeEvent{
		Action:       action,
		ExternalID:   fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		Name:         pod.Name,
		HealthStatus: healthStatus,
		ErrorDetail:  errorDetail,
		Timestamp:    time.Now(),
		Labels:       pod.Labels,
	}
}

func podUpdateEvents(oldPod, newPod *corev1.Pod) []pbruntime.RuntimeEvent {
	var events []pbruntime.RuntimeEvent

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

		events = append(events, pbruntime.RuntimeEvent{
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
		events = append(events, pbruntime.RuntimeEvent{
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

func deploymentToEvent(action string, dep *appsv1.Deployment) pbruntime.RuntimeEvent {
	state, errorDetail := deploymentState(dep)
	_ = state
	replicas := int32(1)
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}
	healthStatus := ""
	if dep.Status.ReadyReplicas >= replicas && replicas > 0 {
		healthStatus = "healthy"
	} else if dep.Status.ReadyReplicas > 0 {
		healthStatus = "starting"
	} else if replicas > 0 {
		healthStatus = "unhealthy"
	}

	return pbruntime.RuntimeEvent{
		Action:       action,
		ExternalID:   fmt.Sprintf("%s/Deployment/%s", dep.Namespace, dep.Name),
		Name:         dep.Name,
		HealthStatus: healthStatus,
		ErrorDetail:  errorDetail,
		Timestamp:    time.Now(),
		Labels:       dep.Labels,
	}
}

func statefulSetToEvent(action string, ss *appsv1.StatefulSet) pbruntime.RuntimeEvent {
	replicas := int32(1)
	if ss.Spec.Replicas != nil {
		replicas = *ss.Spec.Replicas
	}
	healthStatus := ""
	if ss.Status.ReadyReplicas >= replicas && replicas > 0 {
		healthStatus = "healthy"
	} else if ss.Status.ReadyReplicas > 0 {
		healthStatus = "starting"
	} else if replicas > 0 {
		healthStatus = "unhealthy"
	}

	return pbruntime.RuntimeEvent{
		Action:       action,
		ExternalID:   fmt.Sprintf("%s/StatefulSet/%s", ss.Namespace, ss.Name),
		Name:         ss.Name,
		HealthStatus: healthStatus,
		Timestamp:    time.Now(),
		Labels:       ss.Labels,
	}
}

func daemonSetToEvent(action string, ds *appsv1.DaemonSet) pbruntime.RuntimeEvent {
	healthStatus := ""
	if ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
		healthStatus = "healthy"
	} else if ds.Status.NumberReady > 0 {
		healthStatus = "starting"
	} else if ds.Status.DesiredNumberScheduled > 0 {
		healthStatus = "unhealthy"
	}

	return pbruntime.RuntimeEvent{
		Action:       action,
		ExternalID:   fmt.Sprintf("%s/DaemonSet/%s", ds.Namespace, ds.Name),
		Name:         ds.Name,
		HealthStatus: healthStatus,
		Timestamp:    time.Now(),
		Labels:       ds.Labels,
	}
}
