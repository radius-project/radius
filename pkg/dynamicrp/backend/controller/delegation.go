// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/processor"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/delegation"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/schema"
	ucpmodel "github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// This serializes backend execution only, not admission, and only in one process.
// Production requires the durable target aggregate described in the design.
var prototypeDelegationLock sync.Mutex

const bindingType = "Radius.Internal/delegationBindings"

type delegationBinding struct {
	TargetID        string                    `json:"targetId"`
	TargetCreatedAt string                    `json:"targetCreatedAt"`
	ApplicationID   string                    `json:"applicationId"`
	EnvironmentID   string                    `json:"environmentId"`
	Deleting        bool                      `json:"deleting"`
	Reference       recipes.IncomingReference `json:"reference"`
}

func bindingID(resourceID string) (string, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return "", err
	}
	name, err := delegation.Revision(strings.ToLower(id.String()))
	if err != nil {
		return "", err
	}
	return id.RootScope() + "/providers/" + bindingType + "/" + name, nil
}

func metadataFor(resource *datamodel.DynamicResource) recipes.ResourceMetadata {
	return recipes.ResourceMetadata{
		Name: resource.GetRecipe().Name, ResourceID: resource.ID,
		ApplicationID: resource.ResourceMetadata().ApplicationID(),
		EnvironmentID: resource.ResourceMetadata().EnvironmentID(),
	}
}

func (c *DynamicResourceController) runWithDelegation(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	prototypeDelegationLock.Lock()
	defer prototypeDelegationLock.Unlock()

	result, err := c.runDelegationCore(ctx, request)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{
			Code: "DelegationReconciliationFailed", Message: err.Error(),
		}), nil
	}
	return result, nil
}

func (c *DynamicResourceController) runDelegationCore(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	operation, details, err := c.extractOperationAndResourceTypeDetails(ctx, request)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasCapability(details, ucpmodel.CapabilityManualResourceProvisioning) {
		return c.runStandard(ctx, request)
	}
	resource, err := database.GetResource[datamodel.DynamicResource](ctx, c.DatabaseClient(), request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}
	metadata := metadataFor(resource)
	definition, err := c.configurationLoader.LoadRecipe(ctx, &metadata)
	if err != nil {
		return ctrl.Result{}, err
	}
	if definition.Driver == delegation.Kind {
		return c.runDelegate(ctx, request, resource, definition, operation.Method)
	}
	if len(definition.AcceptsDelegationFrom) == 0 {
		return c.runStandard(ctx, request)
	}
	bindings, err := c.loadBindings(ctx, resource)
	if err != nil {
		return ctrl.Result{}, err
	}
	if operation.Method == v1.OperationDelete {
		if len(bindings) > 0 {
			return ctrl.Result{}, fmt.Errorf("target has delegation bindings; delete delegates successfully before deleting the target")
		}
		desired, err := c.desiredProperties(ctx, resource)
		if err != nil {
			return ctrl.Result{}, err
		}
		options := ctrl.Options{
			DatabaseClient: c.DatabaseClient(), ResourceType: resource.Type,
			UcpClient: c.ucp, KubeClient: c.KubeClient(),
		}
		deleteController, err := NewRecipeDeleteController(options, &delegatingEngine{
			Engine: c.engine, incoming: []recipes.IncomingReference{}, desired: desired,
		}, c.configurationLoader, c.secretMaterializer)
		if err != nil {
			return ctrl.Result{}, err
		}
		return deleteController.Run(ctx, request)
	}
	return c.reconcileTarget(ctx, request, resource, bindings)
}

func (c *DynamicResourceController) runDelegate(ctx context.Context, request *ctrl.Request, resource *datamodel.DynamicResource, definition *recipes.EnvironmentDefinition, method v1.OperationMethod) (ctrl.Result, error) {
	id, err := bindingID(resource.ID)
	if err != nil {
		return ctrl.Result{}, err
	}
	existing, err := database.GetResource[delegationBinding](ctx, c.DatabaseClient(), id)
	if err != nil && !errors.Is(err, &database.ErrNotFound{ID: id}) {
		return ctrl.Result{}, err
	}
	if method == v1.OperationDelete && existing == nil {
		if len(resource.OutputResources()) > 0 {
			return ctrl.Result{}, fmt.Errorf("unbound resource still owns native inventory; refusing delegated deletion")
		}
		// A failed admission which never bound a target has no native cleanup obligation.
		return ctrl.Result{}, c.DatabaseClient().Delete(ctx, resource.ID)
	}
	if method == v1.OperationDelete {
		resource.Properties = maps.Clone(existing.Reference.Properties)
	} else if len(resource.GetRecipe().Parameters) > 0 {
		return ctrl.Result{}, fmt.Errorf("delegates cannot override recipe parameters")
	}
	properties, err := c.desiredProperties(ctx, resource)
	if err != nil {
		return ctrl.Result{}, err
	}
	if method == v1.OperationDelete {
		properties = existing.Reference.Properties
	}
	targetID, err := delegation.SelectTarget(properties, definition.Target)
	if err != nil {
		return ctrl.Result{}, err
	}
	if strings.EqualFold(targetID, resource.ID) {
		return ctrl.Result{}, fmt.Errorf("self delegation is not supported")
	}
	target, err := database.GetResource[datamodel.DynamicResource](ctx, c.DatabaseClient(), targetID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("delegation target is missing; cleanup is incomplete if a binding remains: %w", err)
	}
	sourceID, err := resources.ParseResource(resource.ID)
	if err != nil {
		return ctrl.Result{}, err
	}
	selectedID, err := resources.ParseResource(target.ID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !strings.EqualFold(sourceID.RootScope(), selectedID.RootScope()) ||
		resource.TenantID != target.TenantID ||
		!strings.EqualFold(resource.ResourceMetadata().ApplicationID(), target.ResourceMetadata().ApplicationID()) ||
		!strings.EqualFold(resource.ResourceMetadata().EnvironmentID(), target.ResourceMetadata().EnvironmentID()) {
		return ctrl.Result{}, fmt.Errorf("delegate and target must share tenant, resource group, application, and environment")
	}
	if target.ProvisioningState() != v1.ProvisioningStateSucceeded && target.ProvisioningState() != v1.ProvisioningStateFailed {
		return ctrl.Result{}, fmt.Errorf("target must be settled before delegate operations in this sequential prototype")
	}
	if existing != nil && (existing.TargetID != targetID ||
		existing.TargetCreatedAt != target.SystemData.CreatedAt ||
		(method != v1.OperationDelete && (!strings.EqualFold(existing.ApplicationID, resource.ResourceMetadata().ApplicationID()) ||
			!strings.EqualFold(existing.EnvironmentID, resource.ResourceMetadata().EnvironmentID())))) {
		return ctrl.Result{}, fmt.Errorf("bound target, application, and environment are immutable; delete then recreate")
	}
	targetRequest := *request
	targetRequest.ResourceID = target.ID
	targetRequest.APIVersion = target.UpdatedAPIVersion
	targetRequest.OperationType = v1.OperationType{Type: target.Type, Method: v1.OperationPut}.String()
	_, targetDetails, err := c.extractOperationAndResourceTypeDetails(ctx, &targetRequest)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasCapability(targetDetails, ucpmodel.CapabilityManualResourceProvisioning) {
		return ctrl.Result{}, fmt.Errorf("manual-provisioning targets cannot accept delegation")
	}
	targetMetadata := metadataFor(target)
	targetDefinition, err := c.configurationLoader.LoadRecipe(ctx, &targetMetadata)
	if err != nil {
		return ctrl.Result{}, err
	}
	if targetDefinition.Driver == delegation.Kind || !delegation.Accepts(targetDefinition.AcceptsDelegationFrom, resource.Type) {
		return ctrl.Result{}, fmt.Errorf("selected target does not consent to direct provisioning for this delegate")
	}
	if len(resource.OutputResources()) > 0 {
		return ctrl.Result{}, fmt.Errorf("cannot migrate a native-resource owner to delegated provisioning")
	}
	revision, err := delegation.Revision(properties)
	if err != nil {
		return ctrl.Result{}, err
	}
	binding := &delegationBinding{
		TargetID: targetID, TargetCreatedAt: target.SystemData.CreatedAt,
		ApplicationID: resource.ResourceMetadata().ApplicationID(),
		EnvironmentID: resource.ResourceMetadata().EnvironmentID(),
		Deleting:      method == v1.OperationDelete,
		Reference: recipes.IncomingReference{
			ID: strings.ToLower(resource.ID), Type: resource.Type, APIVersion: resource.UpdatedAPIVersion,
			Revision: revision, Properties: properties,
		},
	}
	if err := c.DatabaseClient().Save(ctx, &database.Object{ID: id, Data: binding}); err != nil {
		return ctrl.Result{}, err
	}
	bindings, err := c.loadBindings(ctx, target)
	if err != nil {
		return ctrl.Result{}, err
	}
	result, err := c.reconcileTarget(ctx, &targetRequest, target, bindings)
	if err != nil || result.ProvisioningState() != v1.ProvisioningStateSucceeded {
		return result, err
	}
	if method == v1.OperationDelete {
		if err := c.DatabaseClient().Delete(ctx, resource.ID); err != nil {
			return ctrl.Result{}, err
		}
		if err := c.DatabaseClient().Delete(ctx, id); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (c *DynamicResourceController) loadBindings(ctx context.Context, target *datamodel.DynamicResource) ([]delegationBinding, error) {
	id, err := resources.ParseResource(target.ID)
	if err != nil {
		return nil, err
	}
	result := []delegationBinding{}
	token := ""
	for {
		page, err := c.DatabaseClient().Query(ctx, database.Query{
			RootScope: id.RootScope(), ResourceType: bindingType,
			Filters: []database.QueryFilter{{Field: "targetId", Value: strings.ToLower(target.ID)}},
		}, database.WithPaginationToken(token))
		if err != nil {
			return nil, err
		}
		for _, object := range page.Items {
			var binding delegationBinding
			if err := object.As(&binding); err != nil {
				return nil, err
			}
			if binding.TargetCreatedAt != target.SystemData.CreatedAt {
				return nil, fmt.Errorf("target incarnation differs from persisted binding")
			}
			result = append(result, binding)
		}
		token = page.PaginationToken
		if token == "" {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Reference.ID < result[j].Reference.ID })
	return result, nil
}

func (c *DynamicResourceController) desiredProperties(ctx context.Context, resource *datamodel.DynamicResource) (map[string]any, error) {
	raw, err := processor.GetSchemaForResourceType(ctx, c.ucp, resource.ID, resource.UpdatedAPIVersion)
	if err != nil {
		return nil, err
	}
	resourceSchema, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("delegation requires a registered properties schema")
	}
	if len(schema.ExtractSensitiveFieldPaths(resourceSchema, "")) > 0 {
		return nil, fmt.Errorf("experimental delegation rejects types with sensitive fields")
	}
	if err := schema.ValidateResourceAgainstSchema(ctx, map[string]any{"properties": resource.Properties}, resourceSchema); err != nil {
		return nil, err
	}
	return delegation.DesiredProperties(resource.Properties, resourceSchema)
}

func (c *DynamicResourceController) reconcileTarget(ctx context.Context, request *ctrl.Request, target *datamodel.DynamicResource, bindings []delegationBinding) (ctrl.Result, error) {
	incoming := []recipes.IncomingReference{}
	for _, binding := range bindings {
		if !binding.Deleting {
			incoming = append(incoming, binding.Reference)
		}
	}
	desired, err := c.desiredProperties(ctx, target)
	if err != nil {
		return ctrl.Result{}, err
	}
	snapshotRevision, err := delegation.Revision(struct {
		Desired  map[string]any
		Incoming []recipes.IncomingReference
	}{desired, incoming})
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := c.saveTargetRevision(ctx, target.ID, snapshotRevision, false); err != nil {
		return ctrl.Result{}, err
	}
	wrapped := &delegatingEngine{Engine: c.engine, incoming: incoming, desired: desired}
	options := ctrl.Options{
		DatabaseClient: c.DatabaseClient(), ResourceType: target.Type,
		UcpClient: c.ucp, KubeClient: c.KubeClient(),
	}
	put, err := NewRecipePutController(options, wrapped, c.configurationLoader, c.secretMaterializer)
	if err != nil {
		return ctrl.Result{}, err
	}
	result, err := put.Run(ctx, request)
	if err != nil || result.ProvisioningState() != v1.ProvisioningStateSucceeded {
		return result, err
	}
	if wrapped.output == nil {
		return ctrl.Result{}, fmt.Errorf("delegation requires an executed recipe, not simulation")
	}
	if err := delegation.ValidateOutputs(incoming, wrapped.output.DelegatedOutputs); err != nil {
		return ctrl.Result{}, err
	}
	currentTarget, err := database.GetResource[datamodel.DynamicResource](ctx, c.DatabaseClient(), target.ID)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentDesired, err := c.desiredProperties(ctx, currentTarget)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentRevision, err := delegation.Revision(struct {
		Desired  map[string]any
		Incoming []recipes.IncomingReference
	}{currentDesired, incoming})
	if err != nil {
		return ctrl.Result{}, err
	}
	if currentRevision != snapshotRevision {
		return ctrl.Result{}, fmt.Errorf("target changed during apply; retry the request")
	}
	// Validate the complete projection before saving any delegate outputs.
	updates := make([]*database.Object, 0, len(incoming))
	for _, output := range wrapped.output.DelegatedOutputs {
		stored, err := c.DatabaseClient().Get(ctx, output.ID)
		if err != nil {
			return ctrl.Result{}, err
		}
		var delegate datamodel.DynamicResource
		if err := stored.As(&delegate); err != nil {
			return ctrl.Result{}, err
		}
		latest, err := c.desiredProperties(ctx, &delegate)
		if err != nil {
			return ctrl.Result{}, err
		}
		revision, err := delegation.Revision(latest)
		if err != nil {
			return ctrl.Result{}, err
		}
		if revision != output.Revision {
			return ctrl.Result{}, fmt.Errorf("delegate changed during apply; retry the request")
		}
		raw, err := processor.GetSchemaForResourceType(ctx, c.ucp, delegate.ID, delegate.UpdatedAPIVersion)
		if err != nil {
			return ctrl.Result{}, err
		}
		delegateSchema, ok := raw.(map[string]any)
		if !ok {
			return ctrl.Result{}, fmt.Errorf("delegate schema must be an object")
		}
		if err := projectDelegatedValues(ctx, &delegate, delegateSchema, output.Values); err != nil {
			return ctrl.Result{}, err
		}
		delegate.Status()["reconciliation"] = map[string]any{"desiredRevision": revision, "appliedRevision": revision, "targetId": target.ID}
		stored.Data = &delegate
		updates = append(updates, stored)
	}
	for _, update := range updates {
		if err := c.DatabaseClient().Save(ctx, update, database.WithETag(update.ETag)); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, c.saveTargetRevision(ctx, target.ID, snapshotRevision, true)
}

func (c *DynamicResourceController) saveTargetRevision(ctx context.Context, id, revision string, applied bool) error {
	stored, err := c.DatabaseClient().Get(ctx, id)
	if err != nil {
		return err
	}
	var target datamodel.DynamicResource
	if err := stored.As(&target); err != nil {
		return err
	}
	status, _ := target.Status()["reconciliation"].(map[string]any)
	if status == nil {
		status = map[string]any{}
	}
	status["desiredRevision"] = revision
	if applied {
		status["appliedRevision"] = revision
	}
	target.Status()["reconciliation"] = status
	stored.Data = &target
	return c.DatabaseClient().Save(ctx, stored, database.WithETag(stored.ETag))
}

type delegatingEngine struct {
	engine.Engine
	incoming []recipes.IncomingReference
	desired  map[string]any
	output   *recipes.RecipeOutput
}

func (e *delegatingEngine) Execute(ctx context.Context, options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
	options.Recipe.IncomingReferences = e.incoming
	options.Recipe.Properties = e.desired
	output, err := e.Engine.Execute(ctx, options)
	e.output = output
	return output, err
}

func (e *delegatingEngine) Delete(ctx context.Context, options engine.DeleteOptions) error {
	options.Recipe.IncomingReferences = e.incoming
	options.Recipe.Properties = e.desired
	return e.Engine.Delete(ctx, options)
}

func projectDelegatedValues(ctx context.Context, resource *datamodel.DynamicResource, resourceSchema map[string]any, values map[string]any) error {
	fields, _ := resourceSchema["properties"].(map[string]any)
	for key := range values {
		field, _ := fields[key].(map[string]any)
		readOnly, _ := field["readOnly"].(bool)
		if !readOnly || isReservedDelegatedOutput(key) {
			return fmt.Errorf("delegated output %q is not an allowed read-only computed property", key)
		}
	}
	if strings.EqualFold(resource.Type, "Radius.Compute/routes") {
		listener, ok := values["listener"].(map[string]any)
		if !ok || listener["hostname"] == nil || listener["port"] == nil || listener["protocol"] == nil {
			return fmt.Errorf("routes require a complete delegated listener")
		}
	}
	projected := maps.Clone(resource.Properties)
	maps.Copy(projected, values)
	if err := schema.ValidateResourceAgainstSchema(ctx, map[string]any{"properties": projected}, resourceSchema); err != nil {
		return fmt.Errorf("invalid delegated computed values: %w", err)
	}
	resource.Properties = projected
	return nil
}

func isReservedDelegatedOutput(key string) bool {
	switch key {
	case "status", "provisioningState", "secrets", "application", "environment", "recipe":
		return true
	default:
		return false
	}
}
