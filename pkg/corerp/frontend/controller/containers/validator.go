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
	"errors"
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubeutil"
)

const (
	manifestErrorFormat      = "%s is allowed, but the manifest includes %d resources"
	unmatchedNameErrorFormat = "%s name %s in manifest does not match resource name %s"
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
	if runtimes != nil && runtimes.Kubernetes != nil {
		if err := validateBaseManifest([]byte(runtimes.Kubernetes.Base), newResource); err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("$.properties.runtimes.base is invalid: %v", err)), nil
		}
	}

	return nil, nil
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
	resourceMap, err := kubeutil.ParseManifest(manifest)
	if err != nil {
		return err
	}

	for k, resources := range resourceMap {
		// Do we need to overwrite the namespace? or returning error?
		for _, resource := range resources {
			meta := resource.(metav1.ObjectMetaAccessor)
			if meta.GetObjectMeta().GetNamespace() != "" {
				return fmt.Errorf("namespace is not allowed in resources: %s", meta.GetObjectMeta().GetNamespace())
			}
		}

		switch k {
		case "deployment":
			if len(resources) != 1 {
				return fmt.Errorf(manifestErrorFormat, "only one Deployment", len(resources))
			}
			deployment, ok := resources[0].(*appv1.Deployment)
			if !ok {
				return errors.New("invalid resource for Deployment")
			}
			if deployment.Name != newResource.Name {
				return fmt.Errorf(unmatchedNameErrorFormat, deployment.Kind, deployment.Name, newResource.Name)
			}

		case "service":
			if len(resources) != 1 {
				return fmt.Errorf(manifestErrorFormat, "only one Service", len(resources))
			}
			srv, ok := resources[0].(*corev1.Service)
			if !ok {
				return errors.New("invalid resource for Service")
			}
			if srv.Name != newResource.Name {
				return fmt.Errorf(unmatchedNameErrorFormat, srv.Kind, srv.Name, newResource.Name)
			}

		case "serviceaccount":
			if len(resources) != 1 {
				return fmt.Errorf(manifestErrorFormat, "only one ServiceAccount", len(resources))
			}
			sa, ok := resources[0].(*corev1.ServiceAccount)
			if !ok {
				return errors.New("invalid resource for ServiceAccount")
			}
			if sa.Name != newResource.Name {
				return fmt.Errorf(unmatchedNameErrorFormat, sa.Kind, sa.Name, newResource.Name)
			}

		// No limitations for ConfigMap and Secret resources.
		case "configmap":
		case "secret":

		default:
			return fmt.Errorf("unsupported resource type %s", k)
		}
	}

	return nil
}
