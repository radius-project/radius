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
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
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

	// Resolve recipe pack references (names or full IDs) to canonical resource IDs and
	// validate them, then persist the normalized IDs so downstream readers always see
	// fully-qualified references.
	normalizedRecipePacks, resp, err := e.resolveAndValidateRecipePacks(ctx, serviceCtx.ResourceID, newResource.Properties.RecipePacks)
	if resp != nil || err != nil {
		return resp, err
	}
	newResource.Properties.RecipePacks = normalizedRecipePacks

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

// resolveAndValidateRecipePacks resolves each recipe pack reference to a canonical
// Radius.Core/recipePacks resource ID, verifies the referenced pack exists, and ensures
// that no two packs define a recipe for the same resource type.
//
// A reference may be either a full resource ID or a bare name (e.g. "postgresPack"). A
// bare name is resolved against the environment's own plane and resource group, so
// authors can link a previously deployed pack without repeating the full path. The
// returned slice contains the canonical full resource IDs to persist.
func (e *CreateOrUpdateEnvironmentv20250801preview) resolveAndValidateRecipePacks(ctx context.Context, envID resources.ID, recipePacks []string) ([]string, rest.Response, error) {
	if len(recipePacks) == 0 {
		return recipePacks, nil, nil
	}

	resolved := make([]string, 0, len(recipePacks))
	// resourceTypeMap stores map[lowercaseResourceType]recipePackID to detect conflicts
	// where two packs register a recipe for the same resource type.
	resourceTypeMap := make(map[string]string)

	for _, ref := range recipePacks {
		packID, resp := resolveRecipePackRef(envID, ref)
		if resp != nil {
			return nil, resp, nil
		}

		// Fail fast if the referenced recipe pack does not exist in the resolved scope.
		obj, err := e.DatabaseClient().Get(ctx, packID)
		if err != nil {
			if !errors.Is(err, &database.ErrNotFound{}) {
				return nil, nil, err
			}
			return nil, rest.NewBadRequestResponse(fmt.Sprintf("Referenced recipe pack %q could not be found (resolved to %q): %v", ref, packID, err)), nil
		}

		recipePack := &datamodel.RecipePack{}
		if err := obj.As(recipePack); err != nil {
			return nil, rest.NewBadRequestResponse(fmt.Sprintf("Failed to parse recipe pack %q: %v", packID, err)), nil
		}

		// Check for conflicting resource types across recipe packs.
		for resourceType := range recipePack.Properties.Recipes {
			resourceTypeKey := strings.ToLower(resourceType)
			if existingPackID, exists := resourceTypeMap[resourceTypeKey]; exists {
				if !strings.EqualFold(existingPackID, packID) {
					return nil, rest.NewConflictResponse(fmt.Sprintf("Resource type '%s' is defined in multiple recipe packs: %s and %s", resourceType, existingPackID, packID)), nil
				}
				continue
			}
			resourceTypeMap[resourceTypeKey] = packID
		}

		resolved = append(resolved, packID)
	}

	return resolved, nil, nil
}

// resolveRecipePackRef converts a single recipe pack reference into a canonical
// Radius.Core/recipePacks resource ID. A full resource ID is validated to be of the
// recipe pack type; a bare name is resolved against the environment's plane and resource
// group (envID.RootScope()). It returns a populated rest.Response on validation failure.
func resolveRecipePackRef(envID resources.ID, ref string) (string, rest.Response) {
	// A full resource ID parses successfully; validate it points at a recipe pack.
	if id, err := resources.ParseResource(ref); err == nil {
		if !strings.EqualFold(id.Type(), datamodel.RecipePackResourceType) {
			return "", rest.NewBadRequestResponse(fmt.Sprintf("Referenced recipe pack %q has type %q; expected %q.", ref, id.Type(), datamodel.RecipePackResourceType))
		}
		return id.String(), nil
	}

	// Otherwise treat the reference as a bare name scoped to the environment.
	if ref == "" || strings.Contains(ref, resources.SegmentSeparator) {
		return "", rest.NewBadRequestResponse(fmt.Sprintf("Invalid recipe pack reference %q: provide a recipe pack name or a %s resource ID.", ref, datamodel.RecipePackResourceType))
	}

	scopeID, err := resources.ParseScope(envID.RootScope())
	if err != nil {
		return "", rest.NewBadRequestResponse(fmt.Sprintf("Could not resolve recipe pack %q within scope %q.", ref, envID.RootScope()))
	}

	packID := scopeID.Append(resources.TypeSegment{Type: datamodel.RecipePackResourceType, Name: ref})
	// Re-parse to reject structurally invalid names (e.g. empty or containing '/').
	if parsed, err := resources.ParseResource(packID.String()); err != nil || !parsed.IsResource() {
		return "", rest.NewBadRequestResponse(fmt.Sprintf("Invalid recipe pack reference %q: provide a recipe pack name or a %s resource ID.", ref, datamodel.RecipePackResourceType))
	}

	return packID.String(), nil
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
