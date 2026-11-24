/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hashmigration

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// LabelAppName is the label that identifies the component of a Radius control-plane pod.
const LabelAppName = "app.kubernetes.io/name"

// The control-plane components that read or write values the migration moves. Each is the name of the Deployment of
// the component, and the LabelAppName label of its pods.
const (
	// AppUCP is UCP, which writes tracked resource entries and reads and writes the data store.
	AppUCP = "ucp"

	// AppApplicationsRP is the Applications RP, which reads and writes the data store, runs Terraform, and deploys
	// containers.
	AppApplicationsRP = "applications-rp"

	// AppDynamicRP is the dynamic RP, which reads and writes the data store and runs Terraform.
	AppDynamicRP = "dynamic-rp"
)

// gateOpenFor is how long a gate that found no outdated pods stays open before it looks again.
const gateOpenFor = 5 * time.Second

// annotationRevision is the annotation in which the Deployment controller records the revision of a Deployment, and of
// the ReplicaSet of each revision.
const annotationRevision = "deployment.kubernetes.io/revision"

// hostname returns the name of the pod the component runs in. It is a variable so tests can replace it.
var hostname = os.Hostname

// Gate decides when a step may move values to their SHA-256 form. Steps check it before a pass, and again before each
// move: a component of an earlier release reads and writes values only under the names it knows, so a value it wrote
// while the migration publishes a copy under the current name could be hidden or lost.
type Gate interface {
	// Wait returns why the step must wait before it moves values, or "" if it may move them now.
	Wait(ctx context.Context) (string, error)
}

// Wait returns why the step must wait for gate, or "" if gate is nil or open.
func Wait(ctx context.Context, gate Gate) (string, error) {
	if gate == nil {
		return "", nil
	}

	return gate.Wait(ctx)
}

// ReleaseGate is a Gate that is closed while a pod of one of its components does not belong to the current revision of
// the Deployment of the component. That is the case during a rolling upgrade or a rollback, until the replaced pods
// have stopped; a terminating pod counts until it is gone.
type ReleaseGate struct {
	newClient func() (kubernetes.Interface, error)
	namespace string
	apps      []string
	now       func() time.Time

	mu        sync.Mutex
	client    kubernetes.Interface
	openUntil time.Time
}

// NewReleaseGate returns a ReleaseGate for the components apps, the Deployments named apps whose pods carry the
// LabelAppName label with the same value, in namespace of the cluster client returns.
func NewReleaseGate(client func() (kubernetes.Interface, error), namespace string, apps ...string) *ReleaseGate {
	return &ReleaseGate{newClient: client, namespace: namespace, apps: apps, now: time.Now}
}

// Wait implements Gate.
func (g *ReleaseGate) Wait(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.now().Before(g.openUntil) {
		return "", nil
	}

	if g.client == nil {
		client, err := g.newClient()
		if err != nil {
			return "", err
		}
		g.client = client
	}

	self, err := hostname()
	if err != nil {
		return "", fmt.Errorf("failed to determine the name of this pod: %w", err)
	}

	pods, err := OutdatedPods(ctx, g.client, g.namespace, self, g.apps...)
	if err != nil {
		return "", err
	} else if len(pods) > 0 {
		return fmt.Sprintf("waiting for pods that run another release of Radius to stop, because they read and write values only under their legacy names: %s", strings.Join(pods, ", ")), nil
	}

	g.openUntil = g.now().Add(gateOpenFor)
	return "", nil
}

// OutdatedPods returns the sorted names of the pods of the components apps in namespace that do not belong to the
// current revision of the Deployment of their component, as they do not during a rolling upgrade or a rollback. A
// component is the Deployment named app, and the pods labeled LabelAppName=app that its ReplicaSets own. A component
// without a Deployment is not installed and has no outdated pods. Until the Deployment controller observed the latest
// change of a Deployment, every pod of the component is outdated.
//
// Revisions, rather than container images, tell the pods apart, so containers that an admission webhook injects or
// changes, such as the sidecar of a service mesh, do not make a pod outdated.
//
// It returns nil when the pod named self does not exist in namespace: the caller then does not run in a Radius
// installation, which has no rolling upgrade to wait for.
func OutdatedPods(ctx context.Context, client kubernetes.Interface, namespace string, self string, apps ...string) ([]string, error) {
	_, err := client.CoreV1().Pods(namespace).Get(ctx, self, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read pod %s/%s: %w", namespace, self, err)
	}

	outdated := []string{}
	for _, app := range apps {
		deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, app, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("failed to read Deployment %s/%s: %w", namespace, app, err)
		}

		current := deployment.Annotations[annotationRevision]
		if deployment.Status.ObservedGeneration < deployment.Generation {
			current = ""
		}

		revisions := revisionsOf{client: client, namespace: namespace, deployment: deployment, revisions: map[string]*string{}}
		options := metav1.ListOptions{LabelSelector: labels.Set{LabelAppName: app}.String(), Limit: 100}
		for {
			list, err := client.CoreV1().Pods(namespace).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("failed to list the pods of %s: %w", app, err)
			}

			for i := range list.Items {
				pod := &list.Items[i]
				if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
					continue
				}

				revision, err := revisions.of(ctx, pod)
				if err != nil {
					return nil, err
				} else if revision != nil && (current == "" || *revision != current) {
					outdated = append(outdated, pod.Name)
				}
			}

			if list.Continue == "" {
				break
			}
			options.Continue = list.Continue
		}
	}

	slices.Sort(outdated)
	return outdated, nil
}

// revisionsOf reads the revisions of the ReplicaSets of a Deployment, each once.
type revisionsOf struct {
	client     kubernetes.Interface
	namespace  string
	deployment *appsv1.Deployment

	// revisions holds the revision of each ReplicaSet by name, or nil if it does not belong to the Deployment.
	revisions map[string]*string
}

// of returns the revision of the ReplicaSet that owns pod, or nil if pod does not belong to the Deployment. The
// revision of a ReplicaSet that no longer exists is "", which is never current.
func (r *revisionsOf) of(ctx context.Context, pod *corev1.Pod) (*string, error) {
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.Kind != "ReplicaSet" {
		return nil, nil
	}

	revision, ok := r.revisions[owner.Name]
	if ok {
		return revision, nil
	}

	replicaSet, err := r.client.AppsV1().ReplicaSets(r.namespace).Get(ctx, owner.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		revision = new("")
	case err != nil:
		return nil, fmt.Errorf("failed to read ReplicaSet %s/%s: %w", r.namespace, owner.Name, err)
	case !metav1.IsControlledBy(replicaSet, r.deployment):
		revision = nil
	default:
		revision = new(replicaSet.Annotations[annotationRevision])
	}

	r.revisions[owner.Name] = revision
	return revision, nil
}
