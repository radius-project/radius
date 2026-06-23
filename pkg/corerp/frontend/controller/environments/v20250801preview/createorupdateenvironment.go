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

package v20250801preview

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/util"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateEnvironmentv20250801preview)(nil)

// CreateOrUpdateEnvironmentv20250801preview is the controller implementation to create or update Radius.Core/environments resource.
type CreateOrUpdateEnvironmentv20250801preview struct {
	ctrl.Operation[*datamodel.Environment_v20250801preview, datamodel.Environment_v20250801preview]
}

// NewCreateOrUpdateEnvironmentv20250801preview creates a new controller for creating or updating a Radius.Core/environments resource.
func NewCreateOrUpdateEnvironmentv20250801preview(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateEnvironmentv20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment_v20250801preview]{
				RequestConverter:  converter.Environment20250801DataModelFromVersioned,
				ResponseConverter: converter.Environment20250801DataModelToVersioned,
			},
		),
	}, nil
}

// Run creates or updates a Radius.Core/environments resource.
func (e *CreateOrUpdateEnvironmentv20250801preview) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if resp, err := e.PrepareResource(ctx, req, newResource, old, etag); resp != nil || err != nil {
		return resp, err
	}

	// Create Query filter to query kubernetes namespace used by the other environment resources.
	if newResource.Properties.Providers != nil && newResource.Properties.Providers.Kubernetes != nil {
		namespace := newResource.Properties.Providers.Kubernetes.Namespace

		// Default to the deployment-target cluster's namespace when the environment
		// does not specify one. In multi-cluster mode this is the namespace pinned by
		// the injected target kubeconfig (falling back to "default"); in single-cluster
		// mode it is "default". Persisting it here ensures recipes deploy into a real
		// namespace rather than the empty string.
		if namespace == "" {
			namespace = kubeutil.TargetClusterDefaultNamespace()
			newResource.Properties.Providers.Kubernetes.Namespace = namespace
		}

		result, err := util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), "properties.providers.kubernetes.namespace", namespace, e.DatabaseClient())
		if err != nil {
			return nil, err
		}

		if len(result.Items) > 0 {
			env := &datamodel.Environment_v20250801preview{}
			if err := result.Items[0].As(env); err != nil {
				return nil, err
			}

			// If a different resource has the same namespace, return a conflict
			// Otherwise, continue and update the resource
			if old == nil || env.ID != old.ID {
				return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", env.ID, namespace)), nil
			}
		}

		// Validate the namespace exists on the cluster the application's resources
		// will deploy to. In multi-cluster mode that is the external cluster named by
		// RADIUS_TARGET_KUBECONFIG, not the control-plane cluster Radius runs on, so
		// the check must run against the deployment-target cluster.
		namespaceClient, err := kubeutil.DeploymentTargetRuntimeClient(e.Options().KubeClient)
		if err != nil {
			return nil, err
		}

		ns := &corev1.Namespace{}
		err = namespaceClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return rest.NewBadRequestResponse(fmt.Sprintf("Namespace '%s' does not exist in the Kubernetes cluster. Please create it before proceeding.", namespace)), nil
			}
			return nil, err
		}
	}

	if resp, err := e.validateRecipePacks(ctx, newResource.Properties.RecipePacks); resp != nil || err != nil {
		return resp, err
	}

	// Validate referenced config resources exist and are of the correct type.
	if newResource.Properties.TerraformConfig != "" {
		if resp := validateConfigRef(ctx, e, newResource.Properties.TerraformConfig, datamodel.TerraformConfigResourceType, "terraformConfig"); resp != nil {
			return resp, nil
		}
	}
	if newResource.Properties.BicepConfig != "" {
		if resp := validateConfigRef(ctx, e, newResource.Properties.BicepConfig, datamodel.BicepConfigResourceType, "bicepConfig"); resp != nil {
			return resp, nil
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return e.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}

// Validate recipe packs ensures that no two recipe packs define recipe for the same resource type.
func (e *CreateOrUpdateEnvironmentv20250801preview) validateRecipePacks(ctx context.Context, recipePacks []string) (rest.Response, error) {
	if len(recipePacks) <= 1 {
		return nil, nil
	}

	// map to store map[resourceType]recipePackID
	resourceTypeMap := make(map[string]string)

	for _, recipePackID := range recipePacks {
		id, err := resources.ParseResource(recipePackID)
		if err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("Invalid recipe pack resource ID: %s", recipePackID)), nil
		}

		// Get the recipe pack resource
		obj, err := e.DatabaseClient().Get(ctx, id.String())
		if err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("Failed to retrieve recipe pack %s: %v", recipePackID, err)), nil
		}

		recipePack := &datamodel.RecipePack{}
		if err := obj.As(recipePack); err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("Failed to parse recipe pack %s: %v", recipePackID, err)), nil
		}

		// Check for conflicting resource types across recipe packs
		for resourceType := range recipePack.Properties.Recipes {
			if existingPackID, exists := resourceTypeMap[resourceType]; exists {
				return rest.NewConflictResponse(fmt.Sprintf("Resource type '%s' is defined in multiple recipe packs: %s and %s", resourceType, existingPackID, recipePackID)), nil
			}
			resourceTypeMap[resourceType] = recipePackID
		}
	}

	return nil, nil
}

// validateConfigRef checks that the referenced resource ID parses, has the
// expected resource type, and exists. It returns a populated rest.Response on
// validation failure or nil on success. propertyName is the user-facing field
// label used in error messages (e.g. "terraformConfig").
//
// Without the type check, any existing resource ID (a recipe pack, an
// application, etc.) would silently pass and the loader would fail at recipe
// execution time with a confusing error.
func validateConfigRef(
	ctx context.Context,
	e *CreateOrUpdateEnvironmentv20250801preview,
	resourceID string,
	expectedType string,
	propertyName string,
) rest.Response {
	id, parseErr := resources.Parse(resourceID)
	if parseErr != nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Invalid %s resource ID: %s", propertyName, resourceID))
	}
	if !strings.EqualFold(id.Type(), expectedType) {
		return rest.NewBadRequestResponse(fmt.Sprintf("Referenced %s resource %q has type %q; expected %q.", propertyName, resourceID, id.Type(), expectedType))
	}
	// Operation.GetResource clears the error and returns a nil resource on
	// not-found, so check both. err covers transport/decode failures; the
	// nil out covers the resource-missing case.
	out, _, err := e.GetResource(ctx, id)
	if err != nil {
		return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: fmt.Sprintf("Failed to look up referenced %s resource %q: %s", propertyName, resourceID, err.Error()),
			},
		})
	}
	if out == nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Referenced %s resource %q does not exist.", propertyName, resourceID))
	}
	return nil
}
