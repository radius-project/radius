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

package kubernetes

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// FindDeployment finds deployment in a list of output resources
func FindDeployment(resources []rpv1.OutputResource) (*appsv1.Deployment, rpv1.OutputResource) {
	for _, r := range resources {
		if r.ResourceType.Type != resourcekinds.Deployment {
			continue
		}

		deployment, ok := r.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		return deployment, r
	}

	return nil, rpv1.OutputResource{}
}

// FindService finds service in a list of output resources
func FindService(resources []rpv1.OutputResource) (*corev1.Service, rpv1.OutputResource) {
	for _, r := range resources {
		if r.ResourceType.Type != resourcekinds.Service {
			continue
		}

		service, ok := r.Resource.(*corev1.Service)
		if !ok {
			continue
		}

		return service, r
	}

	return nil, rpv1.OutputResource{}
}

// FindSecret finds secret in a list of output resources
func FindSecret(resources []rpv1.OutputResource) (*corev1.Secret, rpv1.OutputResource) {
	for _, r := range resources {
		if r.ResourceType.Type != resourcekinds.Secret {
			continue
		}

		secret, ok := r.Resource.(*corev1.Secret)
		if !ok {
			continue
		}

		return secret, r
	}

	return nil, rpv1.OutputResource{}
}

// FindHttpRouteByLocalID finds a (non-root) HTTPProxy in a list of output resources, keyed by its localID
func FindHttpRouteByLocalID(resources []rpv1.OutputResource, localID string) (*contourv1.HTTPProxy, rpv1.OutputResource) {
	for _, r := range resources {
		if r.ResourceType.Type != resourcekinds.KubernetesHTTPRoute || r.LocalID != localID {
			continue
		}

		httpRoute, ok := r.Resource.(*contourv1.HTTPProxy)
		if !ok {
			continue
		}

		// If VirtualHost exists, then this is a root HTTPProxy (gateway)
		if httpRoute.Spec.VirtualHost != nil {
			continue
		}

		return httpRoute, r
	}

	return nil, rpv1.OutputResource{}
}

// FindGateway finds a root HTTPProxy in a list of output resources
func FindGateway(resources []rpv1.OutputResource) (*contourv1.HTTPProxy, rpv1.OutputResource) {
	for _, r := range resources {
		if r.ResourceType.Type != resourcekinds.Gateway {
			continue
		}

		gateway, ok := r.Resource.(*contourv1.HTTPProxy)
		if !ok {
			continue
		}

		// If VirtualHost exists, then this is a root HTTPProxy (gateway)
		if gateway.Spec.VirtualHost == nil {
			continue
		}

		return gateway, r
	}

	return nil, rpv1.OutputResource{}
}

// GetShortenedTargetPortName is used to generate a unique port name based on a resource id.
// This is used to link up the a Service and Deployment.
func GetShortenedTargetPortName(name string) string {
	// targetPort can only be a maximum of 15 characters long.
	// 32 bit number should always be less than that.
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return "a" + fmt.Sprint(h.Sum32())
}

// IsValidObjectName returns true if name is valid Kubernetes object name
func IsValidObjectName(name string) bool {
	return len(validation.IsDNS1123Label(name)) == 0
}

func IsValidDaprObjectName(name string) bool {
	return len(validation.IsDNS1123Subdomain(name)) == 0
}
