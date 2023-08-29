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

	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	resources_kubernetes "github.com/project-radius/radius/pkg/ucp/resources/kubernetes"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// FindDeployment searches through a slice of OutputResource objects and returns the first Deployment object and its
// associated OutputResource object.
func FindDeployment(resources []rpv1.OutputResource) (*appsv1.Deployment, rpv1.OutputResource) {
	for _, r := range resources {
		if r.GetResourceType().Type != resources_kubernetes.ResourceTypeDeployment {
			continue
		}

		deployment, ok := r.CreateResource.Data.(*appsv1.Deployment)
		if !ok {
			continue
		}

		return deployment, r
	}

	return nil, rpv1.OutputResource{}
}

// FindService searches through a slice of OutputResource objects and returns the first Service object found and the
// OutputResource object it was found in.
func FindService(resources []rpv1.OutputResource) (*corev1.Service, rpv1.OutputResource) {
	for _, r := range resources {
		if r.GetResourceType().Type != resources_kubernetes.ResourceTypeService {
			continue
		}

		service, ok := r.CreateResource.Data.(*corev1.Service)
		if !ok {
			continue
		}

		return service, r
	}

	return nil, rpv1.OutputResource{}
}

// FindSecret iterates through a slice of OutputResource objects and returns the first Secret object found and its
// corresponding OutputResource object.
func FindSecret(resources []rpv1.OutputResource) (*corev1.Secret, rpv1.OutputResource) {
	for _, r := range resources {
		resourceType := r.GetResourceType()
		if resourceType.Type != resources_kubernetes.ResourceTypeSecret {
			continue
		}

		secret, ok := r.CreateResource.Data.(*corev1.Secret)
		if !ok {
			continue
		}

		return secret, r
	}

	return nil, rpv1.OutputResource{}
}

// FindContourHTTPProxyByLocalID searches through a slice of OutputResources to find a HTTPProxy resource.
func FindContourHTTPProxy(resources []rpv1.OutputResource) (*contourv1.HTTPProxy, rpv1.OutputResource) {
	for _, r := range resources {
		if r.GetResourceType().Type != resources_kubernetes.ResourceTypeContourHTTPProxy {
			continue
		}

		httpProxy, ok := r.CreateResource.Data.(*contourv1.HTTPProxy)
		if !ok {
			continue
		}

		// If VirtualHost exists, then this is a root HTTPProxy (gateway)
		if httpProxy.Spec.VirtualHost == nil {
			continue
		}

		return httpProxy, r
	}

	return nil, rpv1.OutputResource{}
}

// FindContourHTTPProxyByLocalID searches through a slice of OutputResources to find a HTTPProxy resource
// with the given localID.
func FindContourHTTPProxyByLocalID(resources []rpv1.OutputResource, localID string) (*contourv1.HTTPProxy, rpv1.OutputResource) {
	for _, r := range resources {
		if r.GetResourceType().Type != resources_kubernetes.ResourceTypeContourHTTPProxy || r.LocalID != localID {
			continue
		}

		httpRoute, ok := r.CreateResource.Data.(*contourv1.HTTPProxy)
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

// GetShortenedTargetPortName takes in a string and returns a shortened version of it by using a hashing algorithm.
// This generates a unique port name based on a resource id and can be used to link up a Service and Deployment.
func GetShortenedTargetPortName(name string) string {
	// targetPort can only be a maximum of 15 characters long.
	// 32 bit number should always be less than that.
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return "a" + fmt.Sprint(h.Sum32())
}

// IsValidObjectName checks if the given string is a valid Kubernetes object name.
func IsValidObjectName(name string) bool {
	return len(validation.IsDNS1123Label(name)) == 0
}

// IsValidDaprObjectName checks if the given string is a valid Dapr object name and returns a boolean value.
func IsValidDaprObjectName(name string) bool {
	return len(validation.IsDNS1123Subdomain(name)) == 0
}
