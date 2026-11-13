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
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	sm "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpmodel "github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func admitDelegatingResources(ctx context.Context, target, old *datamodel.DynamicResource, definition *recipes.EnvironmentDefinition, targetMetadata recipes.ResourceMetadata,
	loader configloader.ConfigurationLoader, ucp *v20231001preview.ClientFactory, handler *encryption.SensitiveDataHandler, options *controller.Options,
) (rest.Response, error) {
	if definition.Driver == "delegated" || len(definition.AcceptsDelegationFrom) == 0 {
		return rest.NewBadRequestResponse("only a provisioning recipe accepting delegation can receive delegatingResources"), nil
	}
	if options == nil || options.DatabaseClient == nil {
		return nil, fmt.Errorf("group admission requires the resource database")
	}
	request := v1.ARMRequestContextFromContext(ctx)
	members := make([]datamodel.DynamicResource, 0, len(target.DelegatingResources))
	seen := map[string]bool{}
	for _, declaration := range target.DelegatingResources {
		id, err := resources.ParseResource(strings.ToLower(declaration.ID))
		if err != nil || !strings.EqualFold(id.Type(), declaration.Type) || declaration.APIVersion == "" || declaration.Properties == nil {
			return rest.NewBadRequestResponse("each delegating resource requires a valid typed ID, API version and properties"), nil
		}
		key := strings.ToLower(id.String())
		if seen[key] {
			return rest.NewBadRequestResponse("delegatingResources contains duplicate IDs"), nil
		}
		seen[key] = true
		member := datamodel.DynamicResource{Properties: declaration.Properties}
		member.ID, member.Name, member.Type = id.String(), id.Name(), declaration.Type
		member.Location, member.Tags = declaration.Location, declaration.Tags
		member.TenantID, member.UpdatedAPIVersion = target.TenantID, declaration.APIVersion
		provider, name, _ := strings.Cut(id.Type(), "/")
		details, err := ucp.NewResourceTypesClient().Get(ctx, id.FindScope("radius"), provider, name, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to load delegating resource capabilities: %w", err)
		}
		for _, capability := range details.Properties.Capabilities {
			if capability != nil && *capability == ucpmodel.CapabilityManualResourceProvisioning {
				return rest.NewBadRequestResponse("a manual resource cannot delegate provisioning"), nil
			}
		}
		sourceSchema, err := schema.GetSchema(ctx, ucp, member.ID, member.Type, declaration.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to load delegating resource schema: %w", err)
		}
		schema.ApplyDefaults(member.Properties, sourceSchema)
		metadata := recipes.ResourceMetadata{ResourceID: member.ID, EnvironmentID: member.ResourceMetadata().EnvironmentID(),
			ApplicationID: member.ResourceMetadata().ApplicationID(), Name: member.GetRecipe().Name}
		sourceDefinition, err := loader.LoadRecipe(ctx, &metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to load delegating resource registration: %w", err)
		}
		if sourceDefinition.Driver != "delegated" || len(sourceDefinition.AcceptsDelegationFrom) > 0 || len(member.GetRecipe().Parameters) > 0 {
			return rest.NewBadRequestResponse("a group member requires a delegated recipe without source parameters or delegation chaining"), nil
		}
		selected, err := datamodel.SelectDelegationTarget(member.Properties, sourceSchema, sourceDefinition.DelegatesTo)
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
		if !strings.EqualFold(selected.String(), targetMetadata.ResourceID) {
			return rest.NewBadRequestResponse("every delegatingResources reference must select this target"), nil
		}
		previousBinding := ""
		existing, err := options.DatabaseClient.Get(ctx, member.ID)
		if err == nil {
			var previous datamodel.DynamicResource
			if err := existing.As(&previous); err != nil {
				return nil, err
			}
			if !strings.EqualFold(previous.TenantID, target.TenantID) || previous.DelegationTargetID == "" {
				return rest.NewBadRequestResponse("an existing nondelegating resource cannot become a group member"), nil
			}
			previousBinding = previous.DelegationTargetID
		} else if !errors.Is(err, &database.ErrNotFound{ID: member.ID}) {
			return nil, err
		}
		if err := recipes.ValidateDelegationBinding(metadata, targetMetadata, definition, previousBinding); err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
		snapshot, err := datamodel.NewDesiredState(ctx, recipes.IncomingReference{ID: member.ID, Type: member.Type,
			APIVersion: declaration.APIVersion, Revision: request.OperationID.String(), Properties: member.Properties}, sourceSchema, handler)
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
		member.DesiredState, member.DelegationTargetID = snapshot, targetMetadata.ResourceID
		// Pending members contain no supplied properties. Only protected snapshots cross storage.
		member.Properties = map[string]any{}
		members = append(members, member)
		if !slices.ContainsFunc(target.DelegationResourceTypes, func(value string) bool { return strings.EqualFold(value, member.Type) }) {
			target.DelegationResourceTypes = append(target.DelegationResourceTypes, member.Type)
		}
	}
	target.DelegatingDesiredStates = &members
	target.DelegatingResources = nil
	return nil, nil
}

func delegationBusy(resource *datamodel.DynamicResource) bool {
	return resource != nil && resource.DelegationOperationID != ""
}

func makeDelegationUpdateFilter() controller.UpdateFilter[datamodel.DynamicResource] {
	return func(ctx context.Context, resource, old *datamodel.DynamicResource, options *controller.Options) (rest.Response, error) {
		if resource.DesiredState == nil {
			return nil, nil
		}
		if delegationBusy(old) && !old.DelegationDeleting && old.DelegationExecutionStarted && old.ProvisioningState() == v1.ProvisioningStateFailed &&
			old.DesiredState != nil && old.DesiredState.Revision == old.DelegationOperationID {
			// Recovery uses the original protected inputs and operation identity.
			*resource = *old
			resource.DelegationFailedOperationID = old.DelegationOperationID
			return nil, nil
		}
		if delegationBusy(old) || (old != nil && old.DelegationDeleting) {
			return rest.NewConflictResponse("the delegation target has an active operation"), nil
		}
		if resource.DelegationTargetID != "" {
			return claimDelegationTarget(ctx, resource, options, true)
		}
		resource.DelegationOperationID = v1.ARMRequestContextFromContext(ctx).OperationID.String()
		resource.DelegationExecutionStarted = false
		resource.DelegationFailedOperationID = ""
		resource.DelegationResult = nil
		trackDelegationClaim(ctx, resource.ID)
		return nil, nil
	}
}

func makeDelegationDeleteFilter() controller.DeleteFilter[datamodel.DynamicResource] {
	return func(ctx context.Context, resource *datamodel.DynamicResource, options *controller.Options) (rest.Response, error) {
		if resource.DesiredState == nil {
			return nil, nil
		}
		if delegationBusy(resource) {
			return rest.NewConflictResponse("the delegation target has an active operation"), nil
		}
		if resource.DelegationTargetID != "" {
			response, err := claimDelegationTarget(ctx, resource, options, false)
			if errors.Is(err, &database.ErrNotFound{ID: resource.DelegationTargetID}) {
				return nil, nil
			}
			return response, err
		}
		resource.DelegationOperationID = v1.ARMRequestContextFromContext(ctx).OperationID.String()
		resource.DelegationDeleting = true
		trackDelegationClaim(ctx, resource.ID)
		return nil, nil
	}
}

func claimDelegationTarget(ctx context.Context, source *datamodel.DynamicResource, options *controller.Options, allowRecovery bool) (rest.Response, error) {
	object, err := options.DatabaseClient.Get(ctx, source.DelegationTargetID)
	if err != nil {
		return nil, err
	}
	var target datamodel.DynamicResource
	if err := object.As(&target); err != nil {
		return nil, err
	}
	if delegationBusy(&target) || target.DelegationDeleting {
		if allowRecovery && !target.DelegationDeleting && target.DelegationExecutionStarted && target.ProvisioningState() == v1.ProvisioningStateFailed {
			previousObject, err := options.DatabaseClient.Get(ctx, source.ID)
			if err != nil {
				return nil, err
			}
			var previous datamodel.DynamicResource
			if err := previousObject.As(&previous); err != nil {
				return nil, err
			}
			if previous.DesiredState != nil && previous.DesiredState.Revision == target.DelegationOperationID &&
				strings.EqualFold(previous.DelegationTargetID, target.ID) {
				*source = previous
				target.DelegationFailedOperationID = target.DelegationOperationID
				object.Data = &target
				if err := options.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
					return nil, err
				}
				return nil, nil
			}
		}
		return rest.NewConflictResponse("the delegation target has an active operation"), nil
	}
	target.DelegationOperationID = v1.ARMRequestContextFromContext(ctx).OperationID.String()
	target.DelegationExecutionStarted = false
	target.DelegationFailedOperationID = ""
	target.DelegationResult = nil
	if !slices.ContainsFunc(target.DelegationResourceTypes, func(value string) bool { return strings.EqualFold(value, source.Type) }) {
		target.DelegationResourceTypes = append(target.DelegationResourceTypes, source.Type)
	}
	object.Data = &target
	if err := options.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
		if errors.Is(err, &database.ErrConcurrency{}) {
			return rest.NewConflictResponse("the delegation target changed during admission"), nil
		}
		return nil, err
	}
	trackDelegationClaim(ctx, target.ID)
	return nil, nil
}

type delegationClaimKey struct{}

func trackDelegationClaim(ctx context.Context, id string) {
	if claimed, ok := ctx.Value(delegationClaimKey{}).(*string); ok {
		*claimed = id
	}
}

type delegationQueueTracker struct {
	sm.StatusManager
	queued bool
}

func (s *delegationQueueTracker) QueueAsyncOperation(ctx context.Context, request *v1.ARMRequestContext, options sm.QueueOperationOptions) error {
	err := s.StatusManager.QueueAsyncOperation(ctx, request, options)
	s.queued = err == nil
	return err
}

type delegationAdmissionController struct {
	controller.Controller
	store database.Client
	queue *delegationQueueTracker
}

func newDelegationAdmissionController(options controller.Options, resourceOptions controller.ResourceOptions[datamodel.DynamicResource], deleting bool) (controller.Controller, error) {
	queue := &delegationQueueTracker{StatusManager: options.StatusManager}
	options.StatusManager = queue
	var inner controller.Controller
	var err error
	if deleting {
		inner, err = defaultoperation.NewDefaultAsyncDelete(options, resourceOptions)
	} else {
		inner, err = defaultoperation.NewDefaultAsyncPut(options, resourceOptions)
	}
	if err != nil {
		return nil, err
	}
	return &delegationAdmissionController{Controller: inner, store: options.DatabaseClient, queue: queue}, nil
}

func (c *delegationAdmissionController) Run(ctx context.Context, w http.ResponseWriter, request *http.Request) (rest.Response, error) {
	claimed := ""
	ctx = context.WithValue(ctx, delegationClaimKey{}, &claimed)
	response, err := c.Controller.Run(ctx, w, request)
	if err != nil && !c.queue.queued && claimed != "" {
		object, cleanupErr := c.store.Get(ctx, claimed)
		if cleanupErr == nil {
			var target datamodel.DynamicResource
			cleanupErr = object.As(&target)
			if cleanupErr == nil && target.DelegationOperationID == v1.ARMRequestContextFromContext(ctx).OperationID.String() && !target.DelegationExecutionStarted {
				target.DelegationOperationID = ""
				target.DelegationDeleting = false
				object.Data = &target
				cleanupErr = c.store.Save(ctx, object, database.WithETag(object.ETag))
			}
		}
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to release delegation admission claim: %w", cleanupErr))
		}
	}
	return response, err
}
