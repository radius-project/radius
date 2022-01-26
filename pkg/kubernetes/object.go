// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
func FindHttpRoute(resources []outputresource.OutputResource) (*gatewayv1alpha2.HTTPRoute, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		httpRoute, ok := r.Resource.(*gatewayv1alpha2.HTTPRoute)
		if !ok {
			continue
		}

		return httpRoute, r
	}

	return nil, outputresource.OutputResource{}
}

// FindHttpRoute finds an HttpRoute in a list of output resources
func FindGateway(resources []outputresource.OutputResource) (*gatewayv1alpha2.Gateway, outputresource.OutputResource) {
	for _, r := range resources {
		if r.ResourceKind != resourcekinds.Kubernetes {
			continue
		}

		gateway, ok := r.Resource.(*gatewayv1alpha2.Gateway)
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

// MakeScrapedSecretName creates a Secret scraped from input values passed through
// from the deployment template.
func MakeScrapedSecretName(appName string, resourceKind string, resourceName string) string {
	return strings.ToLower(appName + "-" + resourceKind + "-" + resourceName)
}

func MakeScrapedSecret(resource *unstructured.Unstructured, stringData map[string]string) *corev1.Secret {
	resourceKind := resource.GetKind()
	resourceName := resource.GetAnnotations()[LabelRadiusResource]
	appName := resource.GetAnnotations()[LabelRadiusApplication]

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeScrapedSecretName(appName, resourceKind, resourceName),
			Namespace: resource.GetNamespace(),
			Labels:    MakeDescriptiveLabels(appName, resourceName),
			Annotations: map[string]string{
				AnnotationLocalID: outputresource.LocalIDScrapedSecret,
			},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: stringData,
	}
}
