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
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/environments/v20250801preview/settingsref"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/util"
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

		ns := &corev1.Namespace{}
		err = e.Options().KubeClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
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

	// Validate and update settings references (TerraformSettings and BicepSettings)
	if resp, err := e.validateAndUpdateSettingsReferences(ctx, serviceCtx.ResourceID.String(), newResource, old); resp != nil || err != nil {
		return resp, err
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
			if errors.Is(err, &database.ErrNotFound{}) {
				return rest.NewBadRequestResponse(fmt.Sprintf("Recipe pack not found: %s", recipePackID)), nil
			}
			// Return internal error for other database errors (e.g., connection issues)
			return nil, fmt.Errorf("failed to retrieve recipe pack %s: %w", recipePackID, err)
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

// validateAndUpdateSettingsReferences validates that settings resources exist and updates their ReferencedBy lists.
func (e *CreateOrUpdateEnvironmentv20250801preview) validateAndUpdateSettingsReferences(
	ctx context.Context,
	envID string,
	newResource *datamodel.Environment_v20250801preview,
	old *datamodel.Environment_v20250801preview,
) (rest.Response, error) {
	// Validate terraformSettings exists if specified
	if newResource.Properties.TerraformSettings != "" {
		if resp, err := e.validateSettingsExists(ctx, newResource.Properties.TerraformSettings, "terraformSettings"); resp != nil || err != nil {
			return resp, err
		}
	}

	// Validate bicepSettings exists if specified
	if newResource.Properties.BicepSettings != "" {
		if resp, err := e.validateSettingsExists(ctx, newResource.Properties.BicepSettings, "bicepSettings"); resp != nil || err != nil {
			return resp, err
		}
	}

	// Update ReferencedBy on old settings (remove) and new settings (add)
	if err := e.updateSettingsReferences(ctx, envID, old, newResource); err != nil {
		return nil, err
	}

	return nil, nil
}

// validateSettingsExists checks that a settings resource exists in the database.
// It differentiates between "not found" errors and other database errors.
func (e *CreateOrUpdateEnvironmentv20250801preview) validateSettingsExists(
	ctx context.Context,
	settingsID string,
	settingsType string,
) (rest.Response, error) {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Invalid %s resource ID: %s", settingsType, settingsID)), nil
	}

	_, err = e.DatabaseClient().Get(ctx, id.String())
	if err != nil {
		if errors.Is(err, &database.ErrNotFound{}) {
			return rest.NewBadRequestResponse(fmt.Sprintf("%s resource not found: %s", settingsType, settingsID)), nil
		}
		// Return internal error for other database errors (e.g., connection issues)
		return nil, fmt.Errorf("failed to retrieve %s resource %s: %w", settingsType, settingsID, err)
	}

	return nil, nil
}

// updateSettingsReferences updates the ReferencedBy lists on settings resources when references change.
// It uses the shared settingsref package which handles optimistic concurrency with retry logic.
func (e *CreateOrUpdateEnvironmentv20250801preview) updateSettingsReferences(
	ctx context.Context,
	envID string,
	old *datamodel.Environment_v20250801preview,
	newResource *datamodel.Environment_v20250801preview,
) error {
	// Handle TerraformSettings reference changes
	oldTF := ""
	if old != nil {
		oldTF = old.Properties.TerraformSettings
	}
	newTF := newResource.Properties.TerraformSettings

	if oldTF != newTF {
		if oldTF != "" {
			if err := settingsref.RemoveTerraformSettingsReference(ctx, e.DatabaseClient(), oldTF, envID); err != nil {
				return err
			}
		}
		if newTF != "" {
			if err := settingsref.AddTerraformSettingsReference(ctx, e.DatabaseClient(), newTF, envID); err != nil {
				return err
			}
		}
	}

	// Handle BicepSettings reference changes
	oldBicep := ""
	if old != nil {
		oldBicep = old.Properties.BicepSettings
	}
	newBicep := newResource.Properties.BicepSettings

	if oldBicep != newBicep {
		if oldBicep != "" {
			if err := settingsref.RemoveBicepSettingsReference(ctx, e.DatabaseClient(), oldBicep, envID); err != nil {
				return err
			}
		}
		if newBicep != "" {
			if err := settingsref.AddBicepSettingsReference(ctx, e.DatabaseClient(), newBicep, envID); err != nil {
				return err
			}
		}
	}

	return nil
}
