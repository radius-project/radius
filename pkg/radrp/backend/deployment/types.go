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
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/Azure/radius/pkg/radrp/backend/deployment github.com/Azure/radius/pkg/radrp/backend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	// NOTE: the DeploymentProcessor returns errors but they are just for logging, since it's called
	// asynchronously.

	Deploy(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error
	Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
}

func NewDeploymentProcessor(appmodel model.ApplicationModelV3, db db.RadrpDB, healthChannels *healthcontract.HealthChannels) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, db: db, healthChannels: healthChannels}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel       model.ApplicationModelV3
	db             db.RadrpDB
	healthChannels *healthcontract.HealthChannels
}

func (dp *deploymentProcessor) Deploy(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldOperationID, operationID.ID,
	)

	resourceID := operationID.Truncate()

	logger.Info(fmt.Sprintf("Rendering resource: %s, application: %s", resource.ResourceName, resource.ApplicationName))
	renderer, err := dp.appmodel.LookupRenderer(resourceID.Types[len(resourceID.Types)-1].Type) // Using the last type segment as key
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
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
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	rendererDependencies := map[string]renderers.RendererDependency{}
	for _, dependencyResourceID := range dependencyResourceIDs {
		// Fetch resource from db
		dbDependencyResource, err := dp.db.GetV3Resource(ctx, dependencyResourceID)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}

		rendererDependency := renderers.RendererDependency{
			ResourceID:     dependencyResourceID,
			Definition:     dbDependencyResource.Definition,
			ComputedValues: dbDependencyResource.ComputedValues,
		}

		rendererDependencies[dependencyResourceID.ID] = rendererDependency
	}

	// Render - output resources to be deployed for the radius resource
	rendererOutput, err := renderer.Render(ctx, rendererResource, rendererDependencies)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}
	// Order output resources in deployment dependency order
	orderedOutputResources, err := outputresource.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Get existing state of the resource from database, if it's an existing resource
	existingDBResource, err := dp.db.GetV3Resource(ctx, resourceID)
	if err == db.ErrNotFound {
		// no-op - a resource will only exist if this is an update
	} else if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}
	existingDBOutputResources := existingDBResource.Status.OutputResources

	// Deploy and update the radius resource
	dbOutputResources := []db.OutputResource{}
	// values consumed by other Radius resource types through connections
	computedValues := map[string]interface{}{}
	// Map of localID to properties deployed for each output resource. Consumed by handler of any output resource with dependencies on other output resources
	// Example - CosmosDBAccountName consumed by CosmosDBMongo/SQL handler
	deployedOutputResourceProperties := map[string]map[string]string{}
	for _, outputResource := range orderedOutputResources {
		logger.Info(fmt.Sprintf("Deploying output resource - LocalID: %s, type: %s\n", outputResource.LocalID, outputResource.Type))

		var existingOutputResourceState db.OutputResource
		for _, dbOutputResource := range existingDBOutputResources {
			if dbOutputResource.LocalID == outputResource.LocalID {
				existingOutputResourceState = dbOutputResource
				break
			}
		}

		resourceHandlers, err := dp.appmodel.LookupHandlers(outputResource.Kind)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}

		properties, err := resourceHandlers.ResourceHandler.Put(ctx, &handlers.PutOptions{
			Application:            resource.ApplicationName,
			Component:              resource.ResourceName,
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
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}
		deployedOutputResourceProperties[outputResource.LocalID] = properties

		// Copy deployed output resource property values into corresponding expected computed values
		for k, v := range rendererOutput.ComputedValues {
			if outputResource.LocalID == v.LocalID {
				computedValues[k] = properties[v.PropertyReference]
			}
		}

		// Register health checks for the output resource
		healthResourceDetails := healthcontract.ResourceDetails{
			ResourceID:     outputResource.GetResourceID(),
			ResourceKind:   outputResource.Kind,
			ApplicationID:  resource.ApplicationName,
			ComponentID:    resource.ResourceName,
			SubscriptionID: resource.SubscriptionID,
			ResourceGroup:  resource.ResourceGroup,
		}
		healthID := healthResourceDetails.GetHealthID()
		outputResource.HealthID = healthID
		properties[healthcontract.HealthIDKey] = healthID

		dp.registerOutputResourceForHealthChecks(ctx, healthResourceDetails, healthID, resourceHandlers.HealthHandler.GetHealthOptions(ctx))

		// Build database resource - copy updated properties to Resource field
		dbOutputResource := db.OutputResource{
			LocalID:            outputResource.LocalID,
			HealthID:           outputResource.HealthID,
			ResourceKind:       outputResource.Kind,
			OutputResourceInfo: outputResource.Info,
			Managed:            outputResource.Managed,
			OutputResourceType: outputResource.Type,
			Resource:           properties,
			Status: db.OutputResourceStatus{
				ProvisioningState:        db.Provisioned,
				ProvisioningErrorDetails: "",
			},
		}
		dbOutputResources = append(dbOutputResources, dbOutputResource)
	}

	// Update static values for connections
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	// Persist updated/created resource in the database
	resourceStatus := db.RadiusResourceStatus{
		ProvisioningState: db.Provisioned,
		OutputResources:   dbOutputResources,
	}
	updatedRadiusResource := db.RadiusResource{
		ID:              resource.ID,
		Type:            resource.Type,
		SubscriptionID:  resource.SubscriptionID,
		ResourceGroup:   resource.ResourceGroup,
		ApplicationName: resource.ApplicationName,
		ResourceName:    resource.ResourceName,

		Definition:     resource.Definition,
		ComputedValues: computedValues,

		Status: resourceStatus,

		ProvisioningState: string(rest.SuccededStatus),
	}

	err = dp.db.UpdateV3ResourceStatus(ctx, resourceID, updatedRadiusResource)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Update operation
	dp.updateOperation(ctx, rest.SuccededStatus, operationID, nil /* success */)

	return nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, operationID azresources.ResourceID, resource db.RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldOperationID, operationID.ID,
	)

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

		logger.Info(fmt.Sprintf("Deleting output resource - LocalID: %s, type: %s\n", outputResource.LocalID, outputResource.OutputResourceType))
		err = resourceHandlers.ResourceHandler.Delete(ctx, handlers.DeleteOptions{
			Application:            resource.ApplicationName,
			Component:              resource.ResourceGroup,
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

		healthID := outputResource.Resource.(map[string]string)[healthcontract.HealthIDKey]
		dp.unregisterOutputResourceForHealthChecks(ctx, healthID)
	}

	// Delete resource from database
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

	// Update operation
	dp.updateOperation(ctx, rest.SuccededStatus, operationID, nil /* success */)

	return nil
}

func (dp *deploymentProcessor) registerOutputResourceForHealthChecks(ctx context.Context, resourceDetails healthcontract.ResourceDetails, healthID string, healthCheckOptions healthcontract.HealthCheckOptions) {
	logger := radlogger.GetLogger(ctx)
	logger = logger.WithValues(
		radlogger.LogFieldAppName, resourceDetails.ApplicationID,
		radlogger.LogFieldResourceName, resourceDetails.ResourceID,
	)

	if resourceDetails.ResourceID == "" || resourceDetails.ResourceKind == "" || healthID == "" {
		// This additional check is needed until health check is implemented for all resource types:
		// https://github.com/Azure/radius/issues/827
		return
	}

	resourceInfo := healthcontract.ResourceInfo{
		HealthID:     healthID,
		ResourceID:   resourceDetails.ResourceID,
		ResourceKind: resourceDetails.ResourceKind,
	}
	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: resourceInfo,
		Options:      healthCheckOptions,
	}
	dp.healthChannels.ResourceRegistrationWithHealthChannel <- msg

	logger.Info(fmt.Sprintf("Registered output resource with healthID: %s for health checks", healthID))
}

func (dp *deploymentProcessor) unregisterOutputResourceForHealthChecks(ctx context.Context, healthID string) {
	logger := radlogger.GetLogger(ctx)
	logger.Info("Unregistering resource with the health service...")
	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionUnregister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID: healthID,
		},
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
