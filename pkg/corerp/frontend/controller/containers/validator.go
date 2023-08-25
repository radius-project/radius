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

package containers

import (
	"context"
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubeutil"
)

const (
	manifestTargetProperty = "$.properties.runtimes.kubernetes.base"
)

// ValidateAndMutateRequest checks if the newResource has a user-defined identity and if so, returns a bad request
// response, otherwise it sets the identity of the newResource to the identity of the oldResource if it exists.
func ValidateAndMutateRequest(ctx context.Context, newResource, oldResource *datamodel.ContainerResource, options *controller.Options) (rest.Response, error) {
	if newResource.Properties.Identity != nil {
		return rest.NewBadRequestResponse("User-defined identity in Applications.Core/containers is not supported."), nil
	}

	if oldResource != nil {
		// Identity property is populated during deployment.
		// Model converter will not convert .Properties.Identity to datamodel so that newResource.Properties.Identity is always nil.
		// This will populate the existing identity to new resource to keep the identity info.
		newResource.Properties.Identity = oldResource.Properties.Identity
	}

	runtimes := newResource.Properties.Runtimes
	if runtimes != nil && runtimes.Kubernetes != nil && runtimes.Kubernetes.Base != "" {
		err := validateBaseManifest([]byte(runtimes.Kubernetes.Base), newResource)
		if err != nil {
			return rest.NewBadRequestARMResponse(v1.ErrorResponse{Error: err.(v1.ErrorDetails)}), nil
		}
	}

	return nil, nil
}

func errMultipleResources(typeName string, num int) v1.ErrorDetails {
	return v1.ErrorDetails{
		Code:    v1.CodeInvalidRequestContent,
		Target:  "$.properties.runtimes.kubernetes.base",
		Message: fmt.Sprintf("only one %s is allowed, but the manifest includes %d resources.", typeName, num),
	}
}

func errUnmatchedName(obj runtime.Object, name string) v1.ErrorDetails {
	meta := obj.(metav1.ObjectMetaAccessor)
	typeName := obj.GetObjectKind().GroupVersionKind().Kind
	resourceName := meta.GetObjectMeta().GetName()

	return v1.ErrorDetails{
		Code:    v1.CodeInvalidRequestContent,
		Target:  "$.properties.runtimes.kubernetes.base",
		Message: fmt.Sprintf("%s name %s in manifest does not match resource name %s.", typeName, resourceName, name),
	}
}

// validateBaseManifest deserializes the given YAML manifest and validates the allowed number of resources
// and ensures that the resource names of allowed resources match the name of the container resource.
//
// Allowed resource numbers in the manifest:
// - Deployment : 0-1
// - Service : 0-1
// - ServiceAccount : 0-1
// - ConfigMap : 0-N
// - Secret : 0-N
func validateBaseManifest(manifest []byte, newResource *datamodel.ContainerResource) error {
	errDetails := []v1.ErrorDetails{}

	resourceMap, err := kubeutil.ParseManifest(manifest)
	if err != nil {
		return v1.ErrorDetails{
			Code:    v1.CodeInvalidRequestContent,
			Target:  manifestTargetProperty,
			Message: err.Error(),
		}
	}

	for k, resources := range resourceMap {
		// Currently, it returns error immediately if namespaces in resources are set. We may need to override
		// the namespace of the resources in the manifest.
		for _, resource := range resources {
			meta := resource.(metav1.ObjectMetaAccessor)
			if meta.GetObjectMeta().GetNamespace() != "" {
				errDetails = append(errDetails, v1.ErrorDetails{
					Code:    v1.CodeInvalidRequestContent,
					Target:  manifestTargetProperty,
					Message: fmt.Sprintf("namespace is not allowed in resources: %s.", meta.GetObjectMeta().GetNamespace()),
				})
			}
		}

		switch k {
		case "apps/v1/deployment":
			if len(resources) != 1 {
				errDetails = append(errDetails, errMultipleResources("Deployment", len(resources)))
			}
			deployment := resources[0].(*appv1.Deployment)
			if deployment.Name != newResource.Name {
				errDetails = append(errDetails, errUnmatchedName(deployment, newResource.Name))
			}

		case "/v1/service":
			if len(resources) != 1 {
				errDetails = append(errDetails, errMultipleResources("Service", len(resources)))
			}
			srv := resources[0].(*corev1.Service)
			if srv.Name != newResource.Name {
				errDetails = append(errDetails, errUnmatchedName(srv, newResource.Name))
			}

		case "/v1/serviceaccount":
			if len(resources) != 1 {
				errDetails = append(errDetails, errMultipleResources("ServiceAccount", len(resources)))
			}
			sa := resources[0].(*corev1.ServiceAccount)
			if sa.Name != newResource.Name {
				errDetails = append(errDetails, errUnmatchedName(sa, newResource.Name))
			}

		// No limitations for ConfigMap and Secret resources.
		case "/v1/configmap":
		case "/v1/secret":

		default:
			errDetails = append(errDetails, v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: fmt.Sprintf("%s is not supported.", k),
			})
		}
	}

	if len(errDetails) > 0 {
		return v1.ErrorDetails{
			Code:    v1.CodeInvalidRequestContent,
			Target:  manifestTargetProperty,
			Message: "The manifest includes invalid resources.",
			Details: errDetails,
		}
	}

	return nil
}
