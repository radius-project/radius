// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/environment"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/model"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/radrp/backend/deployment github.com/project-radius/radius/pkg/radrp/backend/deployment DeploymentProcessor

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

func (dp *deploymentProcessor) Deploy(ctx context.Context, operationID azresources.ResourceID, radiusResource db.RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, operationID.ID)
	resourceID := operationID.Truncate()

	// Render
	rendererOutput, azureDependencyIDs, armerr, err := dp.renderResource(ctx, resourceID, radiusResource)
	if err != nil {
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Deploy
	logger.Info(fmt.Sprintf("Deploying radius resource: %s, application: %s", radiusResource.ResourceName, radiusResource.ApplicationName))
	deployedRadiusResource, armerr, err := dp.deployRenderedResources(ctx, resourceID, radiusResource, rendererOutput)
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

	// Any azure dependencies referenced from this radius resource will be persisted in the database `azureResources` collection for read operations
	for _, azureResourceID := range azureDependencyIDs {
		dbAzureResource := db.AzureResource{
			ID:             azureResourceID.ID,
			SubscriptionID: azureResourceID.SubscriptionID,
			ResourceGroup:  azureResourceID.ResourceGroup,
			ResourceName:   azureResourceID.QualifiedName(),
			ResourceKind:   resourcekinds.Azure,
			Type:           azureResourceID.Type(),
			// Add Radius application context, since the Azure resource could belong to a different resource group/subscription outside of application context
			ApplicationName:           radiusResource.ApplicationName,
			ApplicationSubscriptionID: radiusResource.SubscriptionID,
			ApplicationResourceGroup:  radiusResource.ResourceGroup,
			RadiusConnectionIDs:       []string{resourceID.ID},
		}

		_, err = dp.db.UpdateAzureResource(ctx, dbAzureResource)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}
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
		outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
			return err
		}

		logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %q\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType))
		err = outputResourceModel.ResourceHandler.Delete(ctx, handlers.DeleteOptions{
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

		if outputResource.Status.HealthState != healthcontract.HealthStateNotApplicable {
			healthResource := healthcontract.HealthResource{
				Identity:         outputResource.Identity,
				RadiusResourceID: resource.ID,
			}
			dp.unregisterOutputResourceForHealthChecks(ctx, healthResource)
		}
	}

	// Delete all azure resource connections for this radius resource from the database
	errorCode, err := dp.deleteAzureResourceConnectionsFromDB(ctx, resource, resourceID)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    errorCode,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		dp.updateOperation(ctx, rest.FailedStatus, operationID, armerr)
		return err
	}

	// Delete radius resource and update operation in the database
	err = dp.db.DeleteV3Resource(ctx, resourceID)
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

// Retrieve resourceIDs of azure dependencies for the radius resource being deleted
func (dp *deploymentProcessor) deleteAzureResourceConnectionsFromDB(ctx context.Context, radiusResource db.RadiusResource, radiusResourceID azresources.ResourceID) (errorCode string, err error) {
	renderer, armerr, err := dp.getResourceRenderer(radiusResourceID)
	if err != nil {
		return armerr.Code, err
	}

	rendererResource := renderers.RendererResource{
		ApplicationName: radiusResource.ApplicationName,
		ResourceName:    radiusResource.ResourceName,
		ResourceType:    radiusResource.Type,
		Definition:      radiusResource.Definition,
	}

	_, azureDependencyIDs, err := renderer.GetDependencyIDs(ctx, rendererResource)
	if err != nil {
		return armerrors.Invalid, err
	}

	for _, azureResourceID := range azureDependencyIDs {
		azureResource, err := dp.db.GetAzureResource(ctx, radiusResourceID.Truncate(), azureResourceID.QualifiedName(), azureResourceID.Type(),
			azureResourceID.SubscriptionID, azureResourceID.ResourceGroup)
		if err != nil {
			if err == db.ErrNotFound {
				// nothing to delete
				continue
			}

			return armerrors.Internal, err
		}

		// If more than one radius resources are connected to this azure resource, only remove connection id for this radius resource
		// else delete the resource entry
		if len(azureResource.RadiusConnectionIDs) > 1 {
			_, err = dp.db.RemoveAzureResourceConnection(ctx, radiusResource.ApplicationName, radiusResource.ID, azureResourceID.ID)
			if err != nil {
				return armerrors.Internal, err
			}
		} else {
			err = dp.db.DeleteAzureResource(ctx, radiusResource.ApplicationName, azureResourceID.ID)
			if err != nil {
				return armerrors.Internal, err
			}
		}
	}

	return "", nil
}

func (dp *deploymentProcessor) renderResource(ctx context.Context, resourceID azresources.ResourceID, resource db.RadiusResource) (renderers.RendererOutput, []azresources.ResourceID, *armerrors.ErrorDetails, error) {
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Rendering resource: %s, application: %s", resource.ResourceName, resource.ApplicationName))
	renderer, armerr, err := dp.getResourceRenderer(resourceID)
	if err != nil {
		return renderers.RendererOutput{}, nil, armerr, err
	}

	// Build inputs for renderer
	rendererResource := renderers.RendererResource{
		ApplicationName: resource.ApplicationName,
		ResourceName:    resource.ResourceName,
		ResourceType:    resource.Type,
		Definition:      resource.Definition,
	}

	// Get resources that the resource being deployed has connection with.
	radiusDependencyResourceIDs, azureDependencyIDs, err := renderer.GetDependencyIDs(ctx, rendererResource)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, nil, armerr, err
	}

	rendererDependencies, err := dp.fetchDependencies(ctx, radiusDependencyResourceIDs)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, nil, armerr, err
	}

	runtimeOptions, err := dp.getRuntimeOptions(ctx)
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, nil, armerr, err
	}

	rendererOutput, err := renderer.Render(ctx, renderers.RenderOptions{Resource: rendererResource, Dependencies: rendererDependencies, Runtime: runtimeOptions})
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return renderers.RendererOutput{}, nil, armerr, err
	}

	// Check if the output resources have the corresponding provider supported in Radius
	for _, or := range rendererOutput.Resources {
		if or.ResourceType.Provider == "" {
			err = fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return renderers.RendererOutput{}, nil, armerr, err
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			err := fmt.Errorf("Provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type)
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return renderers.RendererOutput{}, nil, armerr, err
		}
	}

	return rendererOutput, azureDependencyIDs, nil, nil
}

func (dp *deploymentProcessor) getResourceRenderer(resourceID azresources.ResourceID) (renderers.Renderer, *armerrors.ErrorDetails, error) {
	radiusResourceModel, err := dp.appmodel.LookupRadiusResourceModel(resourceID.Types[len(resourceID.Types)-1].Type) // Using the last type segment as key
	if err != nil {
		armerr := &armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
			Target:  resourceID.ID,
		}
		return nil, armerr, err
	}

	return radiusResourceModel.Renderer, nil, nil
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
		logger.Info(fmt.Sprintf("Deploying output resource: %v, LocalID: %s, resource type: %q\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType))

		var existingOutputResourceState db.OutputResource
		for _, dbOutputResource := range existingDBOutputResources {
			if dbOutputResource.LocalID == outputResource.LocalID {
				existingOutputResourceState = dbOutputResource
				break
			}
		}

		outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
		if err != nil {
			armerr := &armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
				Target:  resourceID.ID,
			}
			return db.RadiusResource{}, armerr, err
		}

		properties, err := outputResourceModel.ResourceHandler.Put(ctx, &handlers.PutOptions{
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

		if outputResource.Identity.ResourceType == nil {
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
			RadiusResourceID: resource.ID,
		}

		supportsHealthMonitor := true
		if !outputResourceModel.SupportsHealthMonitor(outputResource.ResourceType) {
			// Health state is not applicable to this resource and can be skipped from registering with health service
			logger.Info(fmt.Sprintf("Health state is not applicable for resource type: %q. Skipping registration with health service", outputResource.Identity.ResourceType))
			// Return skipped = true
			supportsHealthMonitor = false
		}

		if supportsHealthMonitor {
			dp.registerOutputResourceForHealthChecks(ctx, healthResource, outputResourceModel.HealthHandler.GetHealthOptions(ctx))
		}

		// Build database resource - copy updated properties to Resource field
		dbOutputResource := db.OutputResource{
			LocalID:             outputResource.LocalID,
			ResourceType:        outputResource.ResourceType,
			Identity:            outputResource.Identity,
			PersistedProperties: properties,
			Status: db.OutputResourceStatus{
				ProvisioningState:        db.Provisioned,
				ProvisioningErrorDetails: "",
			},
		}
		if !supportsHealthMonitor {
			dbOutputResource.Status.HealthState = healthcontract.HealthStateNotApplicable
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

// Returns fully qualified radius resource identifier to RendererDependency map
func (dp *deploymentProcessor) fetchDependencies(ctx context.Context, dependencyResourceIDs []azresources.ResourceID) (map[string]renderers.RendererDependency, error) {
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
			Value:         &v.Value,
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

		if (secretReference.Transformer != resourcemodel.ResourceType{}) {
			outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(secretReference.Transformer)
			if err != nil {
				return nil, err
			} else if outputResourceModel.SecretValueTransformer == nil {
				return nil, fmt.Errorf("could not find a secret transformer for %q", secretReference.Transformer)
			}

			secret, err = outputResourceModel.SecretValueTransformer.Transform(ctx, rendererDependency, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to transform secret %q of dependency resource %q: %W", k, id.ID, err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency db.RadiusResource, reference db.SecretValueReference) (interface{}, error) {
	if reference.Value != nil {
		// The secret reference contains the value itself
		return *reference.Value, nil
	}

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

	if dp.secretClient == nil {
		return nil, errors.New("no Azure credentials provided to fetch secret")

	}
	return dp.secretClient.FetchSecret(ctx, match.Identity, reference.Action, reference.ValueSelector)
}

func (dp *deploymentProcessor) getRuntimeOptions(ctx context.Context) (renderers.RuntimeOptions, error) {
	// Get config from radius-config ConfigMap
	var configMaps corev1.ConfigMapList
	err := dp.k8s.List(ctx, &configMaps, &client.ListOptions{Namespace: "radius-system"})
	if err != nil {
		return renderers.RuntimeOptions{}, fmt.Errorf("failed to look up ConfigMaps: %w", err)
	}

	for _, configMap := range configMaps.Items {
		if configMap.Name == "radius-config" {
			return renderers.RuntimeOptions{
				Environment: configMap.Data[environment.EnvironmentKindKey],
				Gateway: renderers.GatewayOptions{
					PublicIP: configMap.Data[environment.HTTPEndpointKey],
				},
			}, nil
		}
	}

	return renderers.RuntimeOptions{}, nil
}
