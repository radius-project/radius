// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Finds deployment in a list of output resources
func FindDeployment(resources []outputresource.OutputResource) *appsv1.Deployment {
	for _, r := range resources {
		if r.Kind != resourcekinds.Kubernetes {
			continue
		}

		deployment, ok := r.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		return deployment
	}

	return nil
}

// Finds service in a list of output resources
func FindService(resources []outputresource.OutputResource) *corev1.Service {
	for _, r := range resources {
		if r.Kind != resourcekinds.Kubernetes {
			continue
		}

		service, ok := r.Resource.(*corev1.Service)
		if !ok {
			continue
		}

		return service
	}

	return nil
}
