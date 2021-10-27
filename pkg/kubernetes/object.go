// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"fmt"
	"hash/fnv"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// FindDeployment finds deployment in a list of output resources
func FindDeployment(resources []outputresource.OutputResource) (*appsv1.Deployment, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		deployment, ok := r.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		return deployment, r
	}

	return nil, outputresource.OutputResource{}
}

// FindService finds service in a list of output resources
func FindService(resources []outputresource.OutputResource) (*corev1.Service, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		service, ok := r.Resource.(*corev1.Service)
		if !ok {
			continue
		}

		return service, r
	}

	return nil, outputresource.OutputResource{}
}

// FindSecret finds secret in a list of output resources
func FindSecret(resources []outputresource.OutputResource) (*corev1.Secret, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		secret, ok := r.Resource.(*corev1.Secret)
		if !ok {
			continue
		}

		return secret, r
	}

	return nil, outputresource.OutputResource{}
}

// FindIngress finds an Ingress in a list of output resources
func FindIngress(resources []outputresource.OutputResource) (*networkingv1.Ingress, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		ingress, ok := r.Resource.(*networkingv1.Ingress)
		if !ok {
			continue
		}

		return ingress, r
	}

	return nil, outputresource.OutputResource{}
}

// FindHttpRoute finds an HttpRoute in a list of output resources
func FindHttpRoute(resources []outputresource.OutputResource) (*gatewayv1alpha1.HTTPRoute, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		httpRoute, ok := r.Resource.(*gatewayv1alpha1.HTTPRoute)
		if !ok {
			continue
		}

		return httpRoute, r
	}

	return nil, outputresource.OutputResource{}
}

// FindHttpRoute finds an HttpRoute in a list of output resources
func FindGateway(resources []outputresource.OutputResource) (*gatewayv1alpha1.Gateway, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		gateway, ok := r.Resource.(*gatewayv1alpha1.Gateway)
		if !ok {
			continue
		}

		return gateway, r
	}

	return nil, outputresource.OutputResource{}
}

// GetShortenedTargetPortName is used to generate a unique port name based on a resource id.
// This is used to link up the a Service and Deployment.
func GetShortenedTargetPortName(name string) string {
	// targetPort can only be a maximum of 15 characters long.
	// 32 bit number should always be less than that.
	h := fnv.New32a()
	h.Write([]byte(name))
	return "a" + fmt.Sprint(h.Sum32())
}
