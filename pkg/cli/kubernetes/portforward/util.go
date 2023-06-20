/*
Copyright 2023 The Radius Authors.

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

package portforward

import (
	"context"

	"github.com/project-radius/radius/pkg/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	k8sclient "k8s.io/client-go/kubernetes"
)

const (
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

// findStaleReplicaSets finds stale ReplicaSets that we should ignore.
//
// This will list all ReplicaSets that are members of the application, then group them by
// owner (Deployment) and return the ReplicaSets that does not match the provided desiredRevision.
//
// This is useful because we frequently run a port-forward right after completion of a Radius
// deployment. We want to make sure we're port-forwarding to fresh replicas, not the ones
// that are being scaled-down.
func findStaleReplicaSets(ctx context.Context, client k8sclient.Interface, namespace, applicationName, desiredRevision string) (map[string]bool, error) {
	outdated := map[string]bool{}

	req, err := labels.NewRequirement(kubernetes.LabelRadiusApplication, selection.Equals, []string{applicationName})
	if err != nil {
		return nil, err
	}

	sets, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*req).String(),
	})
	if err != nil {
		return nil, err
	}

	grouped := map[string][]appsv1.ReplicaSet{}
	for _, set := range sets.Items {
		owner := findOwningDeployment(set)
		if owner == "" {
			// Owner is not a deployment, just skip.
			continue
		}

		grouped[owner] = append(grouped[owner], set)
	}

	for _, values := range grouped {
		for _, replicaSet := range values {
			revision, ok := replicaSet.Annotations[revisionAnnotation]
			// If the annotation is missing, we assume it's outdated.
			// If the annotation is present, but the revision is not the one we want, we assume it's outdated.
			if !ok || revision != desiredRevision {
				outdated[replicaSet.Name] = true
			}
		}
	}

	return outdated, nil
}

func findOwningDeployment(set appsv1.ReplicaSet) string {
	for _, owner := range set.ObjectMeta.OwnerReferences {
		if owner.Kind == "Deployment" {
			return owner.Name
		}
	}

	return ""
}
