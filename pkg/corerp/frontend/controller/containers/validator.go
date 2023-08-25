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

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubeutil"
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
		result := validateBaseManifest([]byte(runtimes.Kubernetes.Base), newResource)
		if !result.valid {
			return rest.NewBadRequestResponse(fmt.Sprintf("$.properties.runtimes.base is invalid: %s", result)), nil
		}
	}

	return nil, nil
}

type validationResult struct {
	valid      bool
	typeName   string
	errMessage string
}

// Valid returns true if the validation result is valid.
func (v validationResult) Valid() bool {
	return v.valid
}

// String returns the error message of the validation result.
func (v validationResult) String() string {
	if v.valid {
		return "valid manifest"
	} else {
		msg := v.errMessage
		if v.typeName != "" {
			msg += ", Type: " + v.typeName
		}
		return msg
	}
}

func errMultipleResources(typeName string, num int) validationResult {
	return validationResult{
		valid:      false,
		typeName:   typeName,
		errMessage: fmt.Sprintf("only one %s is allowed, but the manifest includes %d resources", typeName, num),
	}
}

func errUnmatchedName(obj runtime.Object, name string) validationResult {
	meta := obj.(metav1.ObjectMetaAccessor)
	typeName := obj.GetObjectKind().GroupVersionKind().Kind
	resourceName := meta.GetObjectMeta().GetName()

	return validationResult{
		valid:      false,
		typeName:   typeName,
		errMessage: fmt.Sprintf("%s name %s in manifest does not match resource name %s", typeName, resourceName, name),
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
func validateBaseManifest(manifest []byte, newResource *datamodel.ContainerResource) validationResult {
	resourceMap, err := kubeutil.ParseManifest(manifest)
	if err != nil {
		return validationResult{errMessage: err.Error()}
	}

	for k, resources := range resourceMap {
		// Currently, it returns error immediately if namespaces in resources are set. We may need to override
		// the namespace of the resources in the manifest.
		for _, resource := range resources {
			meta := resource.(metav1.ObjectMetaAccessor)
			if meta.GetObjectMeta().GetNamespace() != "" {
				return validationResult{
					valid:      false,
					typeName:   "namespace",
					errMessage: fmt.Sprintf("namespace is not allowed in resources: %s", meta.GetObjectMeta().GetNamespace()),
				}
			}
		}

		switch k {
		case "deployment":
			if len(resources) != 1 {
				return errMultipleResources("Deployment", len(resources))
			}
			deployment := resources[0].(*appv1.Deployment)
			if deployment.Name != newResource.Name {
				return errUnmatchedName(deployment, newResource.Name)
			}

		case "service":
			if len(resources) != 1 {
				return errMultipleResources("Service", len(resources))
			}
			srv := resources[0].(*corev1.Service)
			if srv.Name != newResource.Name {
				return errUnmatchedName(srv, newResource.Name)
			}

		case "serviceaccount":
			if len(resources) != 1 {
				return errMultipleResources("ServiceAccount", len(resources))
			}
			sa := resources[0].(*corev1.ServiceAccount)
			if sa.Name != newResource.Name {
				return errUnmatchedName(sa, newResource.Name)
			}

		// No limitations for ConfigMap and Secret resources.
		case "configmap":
		case "secret":

		default:
			return validationResult{
				valid:      false,
				errMessage: fmt.Sprintf("unsupported resource type %s", k),
			}
		}
	}

	return validationResult{valid: true}
}
