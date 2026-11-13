/*
Copyright 2026 The Radius Authors.

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

package frontend

import (
	"context"
	"fmt"
	"maps"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpmodel "github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func makeDesiredStateFilter(
	loader configloader.ConfigurationLoader,
	ucpClient *v20231001preview.ClientFactory,
	handler *encryption.SensitiveDataHandler,
) controller.UpdateFilter[datamodel.DynamicResource] {
	return func(ctx context.Context, resource, old *datamodel.DynamicResource, options *controller.Options) (rest.Response, error) {
		if resource == nil || resource.ResourceMetadata().EnvironmentID() == "" {
			if resource != nil && resource.DelegatingResources != nil {
				return rest.NewBadRequestResponse("delegatingResources requires a recipe environment"), nil
			}
			return nil, nil
		}
		environment, err := resources.ParseResource(resource.ResourceMetadata().EnvironmentID())
		if err != nil {
			return nil, fmt.Errorf("failed to resolve desired-state environment: %w", err)
		}
		if !strings.EqualFold(environment.Type(), "Radius.Core/environments") {
			if resource.DelegatingResources != nil {
				return rest.NewBadRequestResponse("delegatingResources requires a Radius.Core recipe environment"), nil
			}
			return nil, nil
		}
		request := v1.ARMRequestContextFromContext(ctx)
		id := request.ResourceID
		provider, name, _ := strings.Cut(id.Type(), "/")
		details, err := ucpClient.NewResourceTypesClient().Get(ctx, id.FindScope("radius"), provider, name, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to load desired-state resource capabilities: %w", err)
		}
		for _, capability := range details.Properties.Capabilities {
			if capability != nil && *capability == ucpmodel.CapabilityManualResourceProvisioning {
				if resource.DelegatingResources != nil {
					return rest.NewBadRequestResponse("a manual resource cannot accept delegatingResources"), nil
				}
				return nil, nil
			}
		}
		metadata := recipes.ResourceMetadata{
			ResourceID: id.String(), EnvironmentID: resource.ResourceMetadata().EnvironmentID(),
			ApplicationID: resource.ResourceMetadata().ApplicationID(), Name: resource.GetRecipe().Name,
		}
		definition, err := loader.LoadRecipe(ctx, &metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to load desired-state recipe registration: %w", err)
		}
		if definition.Driver != "delegated" && len(definition.AcceptsDelegationFrom) == 0 {
			if old != nil && old.DesiredState != nil {
				return rest.NewBadRequestResponse("delete a delegation participant before changing it to a nondelegating recipe"), nil
			}
			if resource.DelegatingResources != nil {
				return rest.NewBadRequestResponse("the recipe does not accept delegation"), nil
			}
			return nil, nil
		}
		if resource.DelegatingResources != nil {
			if response, err := admitDelegatingResources(ctx, resource, old, definition, metadata, loader, ucpClient, handler, options); response != nil || err != nil {
				return response, err
			}
		} else if old == nil && len(definition.AcceptsDelegationFrom) > 0 {
			return rest.NewBadRequestResponse("an initial accepting target requires an authoritative delegatingResources array"), nil
		}
		sourceSchema, err := schema.GetSchema(ctx, ucpClient, id.String(), id.Type(), request.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to load desired-state schema: %w", err)
		}
		if err := captureDesiredState(ctx, resource, sourceSchema, request.OperationID.String(), handler); err != nil {
			return nil, err
		}
		if old != nil {
			resource.DelegationTargetID = old.DelegationTargetID
			if definition.Driver != "delegated" {
				// Native inventory belongs to the target and must survive authored target-only PUTs.
				resource.Properties["status"] = maps.Clone(old.Status())
				resource.DelegationResourceTypes = append(resource.DelegationResourceTypes, old.DelegationResourceTypes...)
			}
		}
		if definition.Driver == "delegated" {
			targetID, err := datamodel.SelectDelegationTarget(resource.Properties, sourceSchema, definition.DelegatesTo)
			if err != nil {
				return rest.NewBadRequestResponse(err.Error()), nil
			}
			if options == nil || options.DatabaseClient == nil {
				return nil, fmt.Errorf("delegation binding requires the resource database")
			}
			object, err := options.DatabaseClient.Get(ctx, targetID.String())
			if err != nil {
				return nil, fmt.Errorf("failed to load delegation target: %w", err)
			}
			var target datamodel.DynamicResource
			if err := object.As(&target); err != nil {
				return nil, err
			}
			targetProvider, targetType, _ := strings.Cut(targetID.Type(), "/")
			targetDetails, err := ucpClient.NewResourceTypesClient().Get(ctx, targetID.FindScope("radius"), targetProvider, targetType, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to load delegation target capabilities: %w", err)
			}
			for _, capability := range targetDetails.Properties.Capabilities {
				if capability != nil && *capability == ucpmodel.CapabilityManualResourceProvisioning {
					return rest.NewBadRequestResponse("a manual-provisioning resource cannot be a delegation target"), nil
				}
			}
			targetMetadata := recipes.ResourceMetadata{
				ResourceID: target.ID, EnvironmentID: target.ResourceMetadata().EnvironmentID(),
				ApplicationID: target.ResourceMetadata().ApplicationID(), Name: target.GetRecipe().Name,
			}
			targetDefinition, err := loader.LoadRecipe(ctx, &targetMetadata)
			if err != nil {
				return nil, fmt.Errorf("failed to load delegation target recipe: %w", err)
			}
			if !strings.EqualFold(resource.TenantID, target.TenantID) {
				return rest.NewBadRequestResponse("delegation requires the same tenant"), nil
			}
			if err := recipes.ValidateDelegationBinding(metadata, targetMetadata, targetDefinition, resource.DelegationTargetID); err != nil {
				return rest.NewBadRequestResponse(err.Error()), nil
			}
			if len(resource.GetRecipe().Parameters) > 0 {
				return rest.NewBadRequestResponse("a delegating resource cannot override recipe parameters"), nil
			}
			resource.DelegationTargetID = target.ID
		}
		return nil, nil
	}
}

func captureDesiredState(ctx context.Context, resource *datamodel.DynamicResource, sourceSchema map[string]any, revision string, handler *encryption.SensitiveDataHandler) error {
	request := v1.ARMRequestContextFromContext(ctx)
	snapshot, err := datamodel.NewDesiredState(ctx, recipes.IncomingReference{
		ID: strings.ToLower(request.ResourceID.String()), Type: request.ResourceID.Type(), APIVersion: request.APIVersion,
		Revision: revision, Properties: resource.Properties,
	}, sourceSchema, handler)
	if err != nil {
		return fmt.Errorf("failed to capture protected desired state: %w", err)
	}
	resource.DesiredState = snapshot
	return nil
}
