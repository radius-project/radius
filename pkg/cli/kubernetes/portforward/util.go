// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package portforward

import (
	"context"
	"sort"

	"github.com/project-radius/radius/pkg/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	k8sclient "k8s.io/client-go/kubernetes"
)

// findStaleReplicaSets finds stale ReplicaSets that we should ignore.
//
// This will list all ReplicaSets that are members of the application, then group them by
// owner (Deployment) then sort them by timestamp and return the old ones.
//
// This is useful because we frequently run a port-forward right after completion of a Radius
// deployment. We want to make sure we're port-forwarding to fresh replicas, not the ones
// that are being scaled-down.
func findStaleReplicaSets(ctx context.Context, client k8sclient.Interface, namespace string, applicationName string) (map[string]bool, error) {
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
		// Sort in place
		sort.Slice(values, func(i, j int) bool {
			// Sort by CreationTimestamp using name as tiebreaker
			if values[i].CreationTimestamp.Equal(&values[j].CreationTimestamp) {
				return values[i].Name < values[j].Name
			}

			// Newest first
			return !values[i].CreationTimestamp.Before(&values[j].CreationTimestamp)
		})

		// Skip newest, add rest to outdated list
		for _, set := range values[1:] {
			outdated[set.Name] = true
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
