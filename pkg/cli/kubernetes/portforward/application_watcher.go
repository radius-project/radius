// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package portforward

import (
	"context"
	"reflect"

	"github.com/project-radius/radius/pkg/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

// applicationWatcher watches a whole application based on labels.
type applicationWatcher struct {
	Options Options

	done               chan struct{}
	deploymentWatchers map[string]*deploymentWatcher
	staleReplicaSets   map[string]bool
}

// NewApplicationWatcher creates a new applicationWatcher.
func NewApplicationWatcher(options Options) *applicationWatcher {
	return &applicationWatcher{
		Options: options,

		done:               make(chan struct{}),
		deploymentWatchers: map[string]*deploymentWatcher{},
	}
}

// Run runs the applicationWatcher until canceled.
func (aw *applicationWatcher) Run(ctx context.Context) error {
	defer close(aw.done)

	// We use the `radius.dev/application` label to include pods that are part of an application.
	// This can include the user's Radius containers as well as any Kubernetes resources that are labeled
	// as part of the application (eg: something created with a recipe).
	req, err := labels.NewRequirement(kubernetes.LabelRadiusApplication, selection.Equals, []string{aw.Options.ApplicationName})
	if err != nil {
		return err
	}

	aw.staleReplicaSets, err = findStaleReplicaSets(ctx, aw.Options.Client, aw.Options.Namespace, aw.Options.ApplicationName)
	if err != nil {
		return err
	}

	deployments := aw.Options.Client.AppsV1().Deployments(aw.Options.Namespace)
	listOptions := metav1.ListOptions{LabelSelector: labels.NewSelector().Add(*req).String()}

	// Starting a watch will populate the current state as well as give us updates
	//
	// RetryWatcher wraps the normal watch functionality to trigger retries when a watch expires or errors.
	watcher, err := watchtools.NewRetryWatcher("1", &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return deployments.Watch(ctx, listOptions)
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

			deployment, ok := event.Object.(*appsv1.Deployment)
			if !ok {
				continue // Shouldn't happen
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				aw.updated(ctx, deployment)
			case watch.Deleted:
				aw.deleted(ctx, deployment)
			}
		}
	}
}

// updated is called for each deployment that is added or updated.
func (aw *applicationWatcher) updated(ctx context.Context, deployment *appsv1.Deployment) {
	// Ignore anything without match labels, it's probably bogus.
	if deployment.Spec.Selector == nil {
		return
	}

	// There are 3 cases to handle here:
	//
	// - deployment is new: need to create a watcher
	// - deployment is updated but still targets the same pods: do nothing
	// - deployment has changed it's match labels: shut down and restart watcher
	//
	entry, ok := aw.deploymentWatchers[deployment.Name]
	if ok && reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, entry.MatchLabels) {
		// deployment is updated but still targets the same pods: do nothing
		return
	} else if ok {
		// deployment has changed its match labels: shut down and restart watcher
		entry.Cancel()
		entry.Wait()
	}

	// if we get here, it's time to create a new watcher
	ctx, cancel := context.WithCancel(ctx)
	entry = NewDeploymentWatcher(aw.Options, deployment.Spec.Selector.MatchLabels, aw.staleReplicaSets, cancel)

	aw.deploymentWatchers[deployment.Name] = entry

	// watcher will run until canceled with its own event-loop
	go func() { _ = entry.Run(ctx) }()
}

// updated is called for each deployment that is deleted.
func (aw *applicationWatcher) deleted(ctx context.Context, deployment *appsv1.Deployment) {
	entry, ok := aw.deploymentWatchers[deployment.Name]
	if ok {
		entry.Cancel()
		entry.Wait()
		delete(aw.deploymentWatchers, deployment.Name)
	}
}

// Wait will wait for the watcher to shut down and will only return once the watcher
// has processed all notifications.
func (aw *applicationWatcher) Wait() {
	<-aw.done
}
