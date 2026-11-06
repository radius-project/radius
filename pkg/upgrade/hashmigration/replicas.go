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
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// LabelAppName is the label that identifies the component of a Radius control-plane pod.
const LabelAppName = "app.kubernetes.io/name"

// OtherReleasePods returns the names of the pods of the component app in namespace, the pods labeled LabelAppName=app,
// whose container named app runs another image than the same container of the pod named self. During a rolling
// upgrade, they are the replicas of the release being replaced, which can still write values in their legacy form. A
// terminating pod counts, because it can still write; a pod that stopped does not.
//
// It returns nil when the pod named self is not found, for example when the component does not run in Kubernetes, and
// when the component may not list pods.
func OtherReleasePods(ctx context.Context, client kubernetes.Interface, namespace string, app string, self string) ([]string, error) {
	pods := []corev1.Pod{}
	options := metav1.ListOptions{LabelSelector: labels.Set{LabelAppName: app}.String(), Limit: 100}
	for {
		list, err := client.CoreV1().Pods(namespace).List(ctx, options)
		if apierrors.IsForbidden(err) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}

		pods = append(pods, list.Items...)
		if list.Continue == "" {
			break
		}

		options.Continue = list.Continue
	}

	image := ""
	for i := range pods {
		if pods[i].Name == self {
			image = containerImage(&pods[i], app)
		}
	}

	if image == "" {
		return nil, nil
	}

	others := []string{}
	for i := range pods {
		pod := &pods[i]
		if pod.Name == self || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if containerImage(pod, app) != image {
			others = append(others, pod.Name)
		}
	}

	slices.Sort(others)
	return others, nil
}

// containerImage returns the image of the container of pod named name, or "" if the pod has no such container.
func containerImage(pod *corev1.Pod, name string) string {
	for _, container := range pod.Spec.Containers {
		if container.Name == name {
			return container.Image
		}
	}

	return ""
}
