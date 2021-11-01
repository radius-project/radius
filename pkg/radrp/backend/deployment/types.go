// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-openapi/jsonpointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/Azure/radius/pkg/radrp/backend/deployment github.com/Azure/radius/pkg/radrp/backend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	// NOTE: the DeploymentProcessor returns errors but they are just for logging, since it's called
	// asynchronously.

	Deploy(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error
	Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
	FetchSecrets(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) (map[string]interface{}, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, db db.RadrpDB, healthChannels *healthcontract.HealthChannels, secretClient renderers.SecretValueClient, k8s client.Client) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, db: db, healthChannels: healthChannels, secretClient: secretClient, k8s: k8s}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel       model.ApplicationModel
	db             db.RadrpDB
	healthChannels *healthcontract.HealthChannels
	secretClient   renderers.SecretValueClient
	k8s            client.Client
}

func (dp *deploymentProcessor) Deploy(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, operationID.ID)
	resourceID := operationID.Truncate()

	// Render
	rendererOutput, armerr, err := dp.renderResource(ctx, resourceID, resource)
	if err != nil {
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Deploy
	logger.Info(fmt.Sprintf("Deploying radius resource: %s, application: %s", resource.ResourceName, resource.ApplicationName))
	deployedRadiusResource, armerr, err := dp.deployRenderedResources(ctx, resourceID, resource, rendererOutput)
	if err != nil {
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Persist updated/created resource and operation in the database
	err = dp.db.UpdateV3ResourceStatus(ctx, resourceID, deployedRadiusResource)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	dp.updateOperation(ctx, rest.SuccededStatus, operationID, nil /* success */)

	return nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, operationID.ID)
	resourceID := operationID.Truncate()

	// Loop over each output resource and delete in reverse dependency order - resource deployed last should be deleted first
	deployedOutputResources := resource.Status.OutputResources
	for i := len(deployedOutputResources) - 1; i >= 0; i-- {
		outputResource := deployedOutputResources[i]
		resourceHandlers, err := dp.appmodel.LookupHandlers(outputResource.ResourceKind)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}

		logger.Info(fmt.Sprintf("Deleting output resource - LocalID: %s, type: %s\n", outputResource.LocalID, outputResource.ResourceKind))
		err = resourceHandlers.ResourceHandler.Delete(ctx, handlers.DeleteOptions{
			Application:            resource.ApplicationName,
			ResourceName:           resource.ResourceName,
			ExistingOutputResource: &outputResource,
		})
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}

		healthResource := healthcontract.HealthResource{
			ResourceKind:     outputResource.ResourceKind,
			Identity:         outputResource.Identity,
			RadiusResourceID: resource.ID,
		}
		dp.unregisterOutputResourceForHealthChecks(ctx, healthResource)
	}

	// Update resource and operation in the database
	err := dp.db.DeleteV3Resource(ctx, resourceID)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	dp.updateOperation(ctx, rest.SuccededStatus, operationID, nil /* success */)

	return nil
}

func (dp *deploymentProcessor) renderResource(ctx context.Context, resourceID azresources.ResourceID, resource db.RadiusResource) (renderers.RendererOutput, *armerrors.ErrorDetails, error) {
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Rendering resource: %s, application: %s", resource.ResourceName, resource.ApplicationName))
	renderer, err := dp.appmodel.LookupRenderer(resourceID.Types[len(resourceID.Types)-1].Type) // Using the last type segment as key
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, armerr, err
	}

	// Build inputs for renderer
	rendererResource := renderers.RendererResource{
		ApplicationName: resource.ApplicationName,
		ResourceName:    resource.ResourceName,
		ResourceType:    resource.Type,
		Definition:      resource.Definition,
	}

	// Get resources that the resource being deployed has connection with.
	dependencyResourceIDs, err := renderer.GetDependencyIDs(ctx, rendererResource)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, armerr, err
	}

	rendererDependencies, err := dp.fetchDepenendencies(ctx, dependencyResourceIDs)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, armerr, err
	}

	runtimeOptions, err := dp.getRuntimeOptions(ctx)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, armerr, err
	}

	rendererOutput, err := renderer.Render(ctx, renderers.RenderOptions{Resource: rendererResource, Dependencies: rendererDependencies, Runtime: runtimeOptions})
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, armerr, err
	}

	return rendererOutput, nil, nil
}

// Deploys rendered output resources in order of dependencies
// returns deployedRadiusResource - updated radius resource state that should be persisted in the database
func (dp *deploymentProcessor) deployRenderedResources(ctx context.Context, resourceID azresources.ResourceID, resource db.RadiusResource, rendererOutput renderers.RendererOutput) (db.RadiusResource, *armerrors.ErrorDetails, error) {
	logger := radlogger.GetLogger(ctx)

	// Order output resources in deployment dependency order
	orderedOutputResources, err := outputresource.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return db.RadiusResource{}, armerr, err
	}

	// Get current state of the resource from database, if it's an existing resource
	existingDBResource, err := dp.db.GetV3Resource(ctx, resourceID)
	if err == db.ErrNotFound {
		// no-op - a resource will only exist if this is an update
	} else if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return db.RadiusResource{}, armerr, err
	}
	existingDBOutputResources := existingDBResource.Status.OutputResources

	deployedOutputResources := []db.OutputResource{}
	// Values consumed by other Radius resource types through connections
	computedValues := map[string]interface{}{}
	// Map of localID to properties deployed for each output resource. Consumed by handler of any output resource with dependencies on other output resources
	// Example - CosmosDBAccountName consumed by CosmosDBMongo/SQL handler
	deployedOutputResourceProperties := map[string]map[string]string{}
	for _, outputResource := range orderedOutputResources {
		logger.Info(fmt.Sprintf("Deploying output resource - LocalID: %s, type: %s\n", outputResource.LocalID, outputResource.ResourceKind))

		var existingOutputResourceState db.OutputResource
		for _, dbOutputResource := range existingDBOutputResources {
			if dbOutputResource.LocalID == outputResource.LocalID {
				existingOutputResourceState = dbOutputResource
				break
			}
		}

		resourceHandlers, err := dp.appmodel.LookupHandlers(outputResource.ResourceKind)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return db.RadiusResource{}, armerr, err
		}

		properties, err := resourceHandlers.ResourceHandler.Put(ctx, &handlers.PutOptions{
			ApplicationName:        resource.ApplicationName,
			ResourceName:           resource.ResourceName,
			Resource:               &outputResource,
			ExistingOutputResource: &existingOutputResourceState,
			DependencyProperties:   deployedOutputResourceProperties,
		})
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return db.RadiusResource{}, armerr, err
		}
		deployedOutputResourceProperties[outputResource.LocalID] = properties

		if outputResource.Identity.Kind == "" {
			err = fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", outputResource.LocalID)
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return db.RadiusResource{}, armerr, err
		}

		// Copy deployed output resource property values into corresponding expected computed values
		for k, v := range rendererOutput.ComputedValues {
			// A computed value might be a reference to a 'property' returned in preserved properties
			if outputResource.LocalID == v.LocalID && v.PropertyReference != "" {
				computedValues[k] = properties[v.PropertyReference]
				continue
			}

			// A computed value might be a 'pointer' into the deployed resource
			if outputResource.LocalID == v.LocalID && v.JSONPointer != "" {
				pointer, err := jsonpointer.New(v.JSONPointer)
				if err != nil {
					err = fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
					armerr := &armerrors.ErrorDetails{
						Code:    armerrors.Internal,
						Message: err.Error(),
						Target:  resourceID.ID,
					}
					return db.RadiusResource{}, armerr, err
				}

				value, _, err := pointer.Get(outputResource.Resource)
				if err != nil {
					err = fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
					armerr := &armerrors.ErrorDetails{
						Code:    armerrors.Internal,
						Message: err.Error(),
						Target:  resourceID.ID,
					}
					return db.RadiusResource{}, armerr, err
				}
				computedValues[k] = value
			}
		}

		// Register health checks for the output resource
		healthResource := healthcontract.HealthResource{
			Identity:         outputResource.Identity,
			ResourceKind:     outputResource.ResourceKind,
			RadiusResourceID: resource.ID,
		}

		dp.registerOutputResourceForHealthChecks(ctx, healthResource, resourceHandlers.HealthHandler.GetHealthOptions(ctx))

		// Build database resource - copy updated properties to Resource field
		dbOutputResource := db.OutputResource{
			LocalID:             outputResource.LocalID,
			ResourceKind:        outputResource.ResourceKind,
			Identity:            outputResource.Identity,
			Managed:             outputResource.Managed,
			PersistedProperties: properties,
			Status: db.OutputResourceStatus{
				ProvisioningState:        db.Provisioned,
				ProvisioningErrorDetails: "",
			},
		}
		deployedOutputResources = append(deployedOutputResources, dbOutputResource)
	}

	// Update static values for connections
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	resourceStatus := db.RadiusResourceStatus{
		ProvisioningState: db.Provisioned,
		OutputResources:   deployedOutputResources,
	}
	deployedRadiusResource := db.RadiusResource{
		ID:              resource.ID,
		Type:            resource.Type,
		SubscriptionID:  resource.SubscriptionID,
		ResourceGroup:   resource.ResourceGroup,
		ApplicationName: resource.ApplicationName,
		ResourceName:    resource.ResourceName,

		Definition:     resource.Definition,
		ComputedValues: computedValues,
		SecretValues:   convertSecretValues(rendererOutput.SecretValues),

		Status: resourceStatus,

		ProvisioningState: string(rest.SuccededStatus),
	}

	return deployedRadiusResource, nil, nil
}

func (dp *deploymentProcessor) registerOutputResourceForHealthChecks(ctx context.Context, resource healthcontract.HealthResource, healthCheckOptions healthcontract.HealthCheckOptions) {
	logger := radlogger.GetLogger(ctx)

	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action:   healthcontract.ActionRegister,
		Resource: resource,
		Options:  healthCheckOptions,
	}
	dp.healthChannels.ResourceRegistrationWithHealthChannel <- msg

	logger.Info("Registered output resource for health checks", resource.Identity.AsLogValues()...)
}

func (dp *deploymentProcessor) unregisterOutputResourceForHealthChecks(ctx context.Context, resource healthcontract.HealthResource) {
	logger := radlogger.GetLogger(ctx)
	logger.Info("Unregistering resource with the health service...", resource.Identity.AsLogValues()...)
	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action:   healthcontract.ActionUnregister,
		Resource: resource,
	}
	dp.healthChannels.ResourceRegistrationWithHealthChannel <- msg
}

// Retrieves and updates existing database entry for the operation
func (dp *deploymentProcessor) updateOperation(ctx context.Context, status rest.OperationStatus, operationResourceID azresources.ResourceID, armerr *armerrors.ErrorDetails) {
	logger := radlogger.GetLogger(ctx)

	operation, err := dp.db.GetOperationByID(ctx, operationResourceID)
	if err == db.ErrNotFound {
		// Operation entry should have been created in the db before we get here
		logger.Error(err, fmt.Sprintf("Update operation failed - operation with id %s was not found in the database.", operationResourceID.ID))
		return
	} else if err != nil {
		logger.Error(err, "Failed to update the operation in database.")
		return
	}

	operation.EndTime = time.Now().UTC().Format(time.RFC3339)
	operation.PercentComplete = 100
	operation.Status = string(status)
	operation.Error = armerr

	_, err = dp.db.PatchOperationByID(ctx, operationResourceID, operation)
	if err != nil {
		logger.Error(err, "Failed to update the operation in database.")
	}
}

func (dp *deploymentProcessor) fetchDepenendencies(ctx context.Context, dependencyResourceIDs []azresources.ResourceID) (map[string]renderers.RendererDependency, error) {
	rendererDependencies := map[string]renderers.RendererDependency{}
	for _, dependencyResourceID := range dependencyResourceIDs {
		// Fetch resource from db
		dbDependencyResource, err := dp.db.GetV3Resource(ctx, dependencyResourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch dependency resource %q: %w", dependencyResourceID.ID, err)
		}

		dependencyOutputResources := map[string]resourcemodel.ResourceIdentity{}
		for _, outputResource := range dbDependencyResource.Status.OutputResources {
			dependencyOutputResources[outputResource.LocalID] = outputResource.Identity
		}

		// We already have all of the computed values (stored in our database), but we need to look secrets
		// (not stored in our database) and add them to the computed values.
		computedValues := map[string]interface{}{}
		for k, v := range dbDependencyResource.ComputedValues {
			computedValues[k] = v
		}

		secretValues, err := dp.FetchSecrets(ctx, dependencyResourceID, dbDependencyResource)
		if err != nil {
			return nil, err
		}

		for k, v := range secretValues {
			computedValues[k] = v
		}

		rendererDependency := renderers.RendererDependency{
			ResourceID:      dependencyResourceID,
			Definition:      dbDependencyResource.Definition,
			ComputedValues:  computedValues,
			OutputResources: dependencyOutputResources,
		}

		rendererDependencies[dependencyResourceID.ID] = rendererDependency
	}

	return rendererDependencies, nil
}

func convertSecretValues(input map[string]renderers.SecretValueReference) map[string]db.SecretValueReference {
	output := map[string]db.SecretValueReference{}
	for k, v := range input {
		output[k] = db.SecretValueReference{
			LocalID:       v.LocalID,
			Action:        v.Action,
			ValueSelector: v.ValueSelector,
			Transformer:   v.Transformer,
		}
	}

	return output
}

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) (map[string]interface{}, error) {
	// We already have all of the computed values (stored in our database), but we need to look secrets
	// (not stored in our database) and add them to the computed values.
	computedValues := map[string]interface{}{}
	for k, v := range resource.ComputedValues {
		computedValues[k] = v
	}

	rendererDependency := renderers.RendererDependency{
		ResourceID:     id,
		Definition:     resource.Definition,
		ComputedValues: computedValues,
	}

	secretValues := map[string]interface{}{}
	for k, secretReference := range resource.SecretValues {
		secret, err := dp.fetchSecret(ctx, resource, secretReference)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch secret %q of dependency resource %q: %w", k, id.ID, err)
		}

		if secretReference.Transformer != "" {
			transformer, err := dp.appmodel.LookupSecretTransformer(secretReference.Transformer)
			if err != nil {
				return nil, err
			}

			secret, err = transformer.Transform(ctx, rendererDependency, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to transform secret %q of dependency resource %q: %W", k, id.ID, err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency db.RadiusResource, reference db.SecretValueReference) (interface{}, error) {
	var match *db.OutputResource
	for _, outputResource := range dependency.Status.OutputResources {
		if outputResource.LocalID == reference.LocalID {
			copy := outputResource
			match = &copy
			break
		}
	}

	if match == nil {
		return nil, fmt.Errorf("cannot find an output resource matching LocalID %q for dependency %q", reference.LocalID, dependency.ID)
	}

	return dp.secretClient.FetchSecret(ctx, match.Identity, reference.Action, reference.ValueSelector)
}

func (dp *deploymentProcessor) getRuntimeOptions(ctx context.Context) (renderers.RuntimeOptions, error) {
	options := renderers.RuntimeOptions{}
	// We require a gateway class to be present before creating a gateway
	// Look up the first gateway class in the cluster and use that for now
	if dp.k8s != nil {
		var gateways gatewayv1alpha1.GatewayClassList
		err := dp.k8s.List(ctx, &gateways)
		if err != nil {
			// Ignore failures to list gateway classes
			return renderers.RuntimeOptions{}, nil
		}

		if len(gateways.Items) > 0 {
			gatewayClass := gateways.Items[0]
			options.Gateway = renderers.GatewayOptions{
				GatewayClass: gatewayClass.Name,
			}
		}
	}

	return options, nil
}
