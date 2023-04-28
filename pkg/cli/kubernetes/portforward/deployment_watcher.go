// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package portforward

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

type deploymentWatcher struct {
	Cancel           func()
	MatchLabels      map[string]string
	Options          Options
	StaleReplicaSets map[string]bool

	done chan struct{}
	pods map[string]*corev1.Pod

	// We use a single podWatcher because we only want to listen to one replica from a deployment
	// at a time.
	podWatcher *podWatcher
}

func NewDeploymentWatcher(options Options, matchLabels map[string]string, staleReplicaSets map[string]bool, cancel func()) *deploymentWatcher {
	return &deploymentWatcher{
		Cancel:           cancel,
		MatchLabels:      matchLabels,
		Options:          options,
		StaleReplicaSets: staleReplicaSets,

		done: make(chan struct{}),
		pods: map[string]*corev1.Pod{},
	}
}

func (dw *deploymentWatcher) Run(ctx context.Context) error {
	defer close(dw.done)

	// We need to transform the "match labels" format into the format expected by
	// watch.
	selector := labels.NewSelector()
	for name, value := range dw.MatchLabels {
		req, err := labels.NewRequirement(name, selection.Equals, []string{value})
		if err != nil {
			return err
		}

		selector = selector.Add(*req)
	}

	pods := dw.Options.Client.CoreV1().Pods(dw.Options.Namespace)
	listOptions := metav1.ListOptions{LabelSelector: selector.String()}

	// Starting a watch will populate the current state as well as give us updates
	//
	// RetryWatcher wraps the normal watch functionality to trigger retries when a watch expires or errors.
	watcher, err := watchtools.NewRetryWatcher("1", &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return pods.Watch(ctx, listOptions)
		},
	})
	if err != nil {
		return err
	}

	// No synchronization is needed for our data structures as we're executing single-threaded.
	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			return ctx.Err()

		case event := <-watcher.ResultChan():
			if event.Object == nil {
				// This can happen when the watch is shutting down.
				watcher.Stop()
				return nil
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue // Shouldn't happen
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				dw.updated(ctx, pod)
			case watch.Deleted:
				dw.deleted(ctx, pod)
			}
		}
	}
}

func (dw *deploymentWatcher) ignorePod(pod *corev1.Pod) bool {
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			_, found := dw.StaleReplicaSets[owner.Name]
			return found
		}
	}

	return false
}

func (dw *deploymentWatcher) updated(ctx context.Context, pod *corev1.Pod) {
	// The deployment watcher only wants to watch one replica from each deployment.
	// We also need to keep a cache of pods which will help us select a new pod when needed.

	// We have 3 cases to handle for here:
	//
	// - Pod is added
	// - Pod is added but we are ignoring it because it's "stale"
	// - Pod is being deleted
	//
	// The spec of a pod is immutable, which means that we don't need to handle updates.
	//
	// We handle deletion primarily though the deletion timestamp. This is the earliest
	// way for us to be notified of a pod shutdown. Pods have a finalizer so their deletion
	// is two-phase. We want to disconnect early and not wait the ~30 seconds pod deletion
	// may take by default.

	// Update cache
	if pod.DeletionTimestamp != nil {
		// Pod is marked for deletion
		delete(dw.pods, pod.Name)
	} else if dw.ignorePod(pod) {
		// Pod should be ignored, do nothing
	} else {
		// Pod is being added/updated
		dw.pods[pod.Name] = pod
	}

	// There's an additional consideration when the pod that's being changed is the one we're watching.
	//
	// - If the pod we're watching is being deleted then shut down our watch.
	// - If the pod we'ere watching is being updated then notify the pod watcher.
	if dw.podWatcher != nil && dw.podWatcher.Pod.Name == pod.Name && pod.DeletionTimestamp != nil {
		dw.podWatcher.Cancel()
		close(dw.podWatcher.Updated)
		dw.podWatcher.Wait()
		dw.podWatcher = nil
	} else if dw.podWatcher != nil && dw.podWatcher.Pod.Name == pod.Name {
		dw.podWatcher.Updated <- pod
	}

	// Start a new watcher if needed.
	dw.ensureWatcher(ctx)
}

func (dw *deploymentWatcher) deleted(ctx context.Context, pod *corev1.Pod) {
	delete(dw.pods, pod.Name)

	// If the pod we're watching is being deleted then shut down our watch.
	if dw.podWatcher != nil && dw.podWatcher.Pod.Name == pod.Name {
		dw.podWatcher.Cancel()
		close(dw.podWatcher.Updated)
		dw.podWatcher.Wait()
		dw.podWatcher = nil
	}

	dw.ensureWatcher(ctx)
}

func (dw *deploymentWatcher) ensureWatcher(ctx context.Context) {
	if dw.podWatcher == nil && len(dw.pods) > 0 {
		pod := dw.selectBestPod()

		ctx, cancel := context.WithCancel(ctx)
		dw.podWatcher = NewPodWatcher(dw.Options, pod, cancel)

		// will run until canceled
		go func() { _ = dw.podWatcher.Run(ctx) }()
	}
}

func (dw *deploymentWatcher) selectBestPod() *corev1.Pod {
	// We always want to take the newest pod.

	pods := []*corev1.Pod{}
	for _, pod := range dw.pods {
		pods = append(pods, pod)
	}

	// Sort in place
	sort.Slice(pods, func(i, j int) bool {
		// Sort by CreationTimestamp using name as tiebreaker
		if pods[i].CreationTimestamp.Equal(&pods[j].CreationTimestamp) {
			return pods[i].Name < pods[j].Name
		}

		// Newest first
		return !pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
	})

	if len(pods) == 0 {
		return nil
	}

	return pods[0]
}

func (dw *deploymentWatcher) Wait() {
	<-dw.done
}
