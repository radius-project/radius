// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/algorithm/graph"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

//go:generate mockgen -destination=./mock_deployment_processor.go -package=deployment -self_package github.com/Azure/radius/pkg/radrp/deployment github.com/Azure/radius/pkg/radrp/deployment DeploymentProcessor

// DeploymentProcessor implements functionality for updating and deleting deployments.
type DeploymentProcessor interface {
	UpdateDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus, actions map[string]ComponentAction) error
	DeleteDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus) error
	RegisterForHealthChecks(ctx context.Context, appName string, c db.Component) error
}

type deploymentProcessor struct {
	appmodel model.ApplicationModel
	health   *healthcontract.HealthChannels
}

// NewDeploymentProcessor initializes a deployment processor.
func NewDeploymentProcessor(appmodel model.ApplicationModel, health *healthcontract.HealthChannels) DeploymentProcessor {
	return &deploymentProcessor{
		appmodel: appmodel,
		health:   health,
	}
}

func (dp *deploymentProcessor) UpdateDeployment(ctx context.Context, appName string, name string, deploymentStatus *db.DeploymentStatus, actions map[string]ComponentAction) error {
	// TODO - any sort of rollback - we'll leave things in a partially-created state
	// for now if we encounter a failure at any point.
	//
	// TODO - we don't handle the case where resources disappear without the component being
	// deleted. All of the resources we support so far are 1-1 with the components.
	errs := []error{}
	ctx = radlogger.WrapLogContext(ctx, radlogger.LogFieldAppName, appName)
	logger := radlogger.GetLogger(ctx)

	ordered, err := dp.orderActions(actions)
	if err != nil {
		return err
	}

	bindingValues := map[components.BindingKey]components.BindingState{}

	// Process each action and update the deployment status as we go ...
	for i, action := range ordered {
		logger = logger.WithValues(
			radlogger.LogFieldAction, action.Operation,
			radlogger.LogFieldComponentName, action.ComponentName,
		)
		logger.Info(fmt.Sprintf("Executing actions in order: %v - %v", i, action.ComponentName))

		// while we do bookkeeping, also update the deployment record
		switch action.Operation {

		case None:
			// Don't update resources or services - we should already have them from the DB
			//
			// However we should process bindings for these so the values are accessible to
			// other components that need them.
			//
			// We need to fetch the properties for the existing resources from the database
			// in order to do this.

			resources := []workloads.WorkloadResourceProperties{}
			for _, status := range deploymentStatus.Workloads {
				if status.ComponentName != action.Component.Name {
					continue
				}

				for _, resource := range status.Resources {
					wr := workloads.WorkloadResourceProperties{
						LocalID:    resource.LocalID,
						Type:       resource.Type,
						Properties: resource.Properties,
					}
					resources = append(resources, wr)
				}
			}

			workload := workloads.InstantiatedWorkload{
				Application:   appName,
				Name:          action.ComponentName,
				Workload:      *action.Component,
				BindingValues: bindingValues,
			}

			err = dp.processBindings(ctx, workload, resources, bindingValues)
			if err != nil {
				errs = append(errs, fmt.Errorf("error applying bindings for component %v : %w", action.ComponentName, err))
				continue
			}

		case CreateWorkload, UpdateWorkload:
			// For an update, just blow away the existing workload record
			dbDeploymentWorkload := db.DeploymentWorkload{
				ComponentName: action.ComponentName,
				Kind:          action.Definition.Kind,
			}

			workload := workloads.InstantiatedWorkload{
				Application:   appName,
				Name:          action.ComponentName,
				Workload:      *action.Component,
				BindingValues: bindingValues,
			}

			logger.Info(fmt.Sprintf("Rendering workload. Application: %s, Component: %s", workload.Application, workload.Name))
			dbOutputResources := []db.OutputResource{}
			outputResources, err := dp.renderWorkload(ctx, workload)
			if err != nil {
				errs = append(errs, err)
				for _, resource := range outputResources {
					logger.WithValues(radlogger.LogFieldLocalID, resource.LocalID).Info(fmt.Sprintf("Rendered output resource - LocalID: %s, type: %s\n", resource.LocalID, resource.Type))

					// Even if the operation fails, return the output resources created so far
					// TODO: This is temporary. Once there are no resources actually deployed during render phase,
					// we no longer need to track the output resources on error
					addDBOutputResource(resource, &dbOutputResources)
				}
				action.Definition.Properties.Status.OutputResources = dbOutputResources
				continue
			}

			orderedResources, err := outputresource.OrderOutputResources(outputResources)
			if err != nil {
				errs = append(errs, err)
			}

			// Deploy output resources rendered for the workload in order of dependencies
			for _, resource := range orderedResources {
				var existingResourceState *db.DeploymentResource
			workloadsloop:
				for _, existingState := range deploymentStatus.Workloads {
					if existingState.ComponentName == action.ComponentName {
						for _, currentResource := range existingState.Resources {
							if currentResource.LocalID == resource.LocalID {
								existingResourceState = &currentResource
								break workloadsloop
							}
						}
					}
				}

				resourceType, err := dp.appmodel.LookupResource(resource.Kind)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				dependencies := []db.DeploymentResource{}
				// Dependencies that the output resource being deployed depends on are passed to the handler to consume
				for _, dependency := range resource.Dependencies {
					for _, deployedResource := range dbDeploymentWorkload.Resources {
						if deployedResource.LocalID == dependency.LocalID {
							dependencies = append(dependencies, deployedResource)
							// break out of the inner loop once a deployed resource for the dependency is found
							break
						}
					}
				}

				properties, err := resourceType.Handler().Put(ctx, &handlers.PutOptions{
					Application:  appName,
					Component:    action.ComponentName,
					Resource:     &resource,
					Existing:     existingResourceState,
					Dependencies: dependencies,
				})

				outputResourceInfo := healthcontract.ResourceDetails{
					ResourceID:     resource.GetResourceID(),
					ResourceKind:   resource.Kind,
					ApplicationID:  appName,
					ComponentID:    action.ComponentName,
					SubscriptionID: action.Definition.SubscriptionID,
					ResourceGroup:  action.Definition.ResourceGroup,
				}
				// Save the healthID on the resource
				healthID := outputResourceInfo.GetHealthID()
				resource.HealthID = healthID

				if err != nil {
					// Until https://github.com/Azure/radius/issues/614 is resolved, add output resources created so far
					// to the DB
					resource.Status.ProvisioningState = db.Failed
					resource.Status.ProvisioningErrorDetails = err.Error()
					addDBOutputResource(resource, &dbOutputResources)
					action.Definition.Properties.Status.OutputResources = dbOutputResources
					errs = append(errs, fmt.Errorf("error applying workload for component %v %v: %w", properties, action.ComponentName, err))
					continue
				}

				properties[healthcontract.HealthIDKey] = healthID
				resource.Status.ProvisioningState = db.Provisioned
				resource.Status.ProvisioningErrorDetails = ""

				// Persist output resource state in the database.
				addDBOutputResource(resource, &dbOutputResources)
				action.Definition.Properties.Status.OutputResources = dbOutputResources

				dbDeploymentResource := db.DeploymentResource{
					LocalID:    resource.LocalID,
					Type:       resource.Kind,
					Properties: properties,
				}
				dbDeploymentWorkload.Resources = append(dbDeploymentWorkload.Resources, dbDeploymentResource)
			}

			deployedResources := []workloads.WorkloadResourceProperties{}
			for _, resource := range dbDeploymentWorkload.Resources {
				resourceProperties := workloads.WorkloadResourceProperties{
					LocalID:    resource.LocalID,
					Type:       resource.Type,
					Properties: resource.Properties,
				}
				deployedResources = append(deployedResources, resourceProperties)
			}

			// Populate data for the bindings that this component provides
			err = dp.processBindings(ctx, workload, deployedResources, bindingValues)
			if err != nil {
				errs = append(errs, fmt.Errorf("error applying workload bindings %v: %w", action.ComponentName, err))
				continue
			}

			updated := false
			for i, existing := range deploymentStatus.Workloads {
				if existing.ComponentName == dbDeploymentWorkload.ComponentName {
					deploymentStatus.Workloads[i] = dbDeploymentWorkload
					updated = true
					break
				}
			}

			if !updated {
				deploymentStatus.Workloads = append(deploymentStatus.Workloads, dbDeploymentWorkload)
			}

			logger.WithValues(radlogger.LogFieldComponentKind, action.Component.Kind).Info("successfully applied workload")

		case DeleteWorkload:
			// Remove the deployment record
			var workload db.DeploymentWorkload
			for i, existingState := range deploymentStatus.Workloads {
				if existingState.ComponentName == action.ComponentName {
					workload = existingState
					deploymentStatus.Workloads = append(deploymentStatus.Workloads[:i], deploymentStatus.Workloads[i+1:]...)
					break
				}
			}

			if workload.ComponentName == "" {
				errs = append(errs, fmt.Errorf("cannot find deployment record for %v", action.ComponentName))
				continue
			}

			for _, resource := range workload.Resources {
				resourceType, err := dp.appmodel.LookupResource(resource.Type)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				err = resourceType.Handler().Delete(ctx, handlers.DeleteOptions{
					Application: appName,
					Component:   action.ComponentName,
					Existing:    resource,
				})

				// Unregister resource from HealthService for health monitoring
				healthID := resource.Properties[healthcontract.HealthIDKey]
				dp.UnregisterForHealthChecks(ctx, healthID)
				if err != nil {
					errs = append(errs, fmt.Errorf("error deleting workload resource %v %v: %w", resource.Properties, action.ComponentName, err))
					continue
				}
			}

			logger.Info("successfully deleted workload")
		}
	}

	names := map[string]bool{}
	for _, dw := range deploymentStatus.Workloads {
		if _, ok := names[dw.ComponentName]; ok {
			errs = append(errs, fmt.Errorf("duplicate component name %v", dw.ComponentName))
		}

		names[dw.ComponentName] = true
	}

	if len(errs) > 0 {
		return &CompositeError{Errors: errs}
	}

	return nil
}

func addDBOutputResource(resource outputresource.OutputResource, dbOutputResources *[]db.OutputResource) {
	// Save the output resource to DB
	dbr := db.OutputResource{
		ResourceID:         resource.GetResourceID(),
		Managed:            resource.Managed,
		HealthID:           resource.HealthID,
		LocalID:            resource.LocalID,
		ResourceKind:       resource.Kind,
		OutputResourceType: resource.Type,
		OutputResourceInfo: resource.Info,
		Resource:           resource.Resource,
		Status: db.OutputResourceStatus{
			ProvisioningState:        resource.Status.ProvisioningState,
			ProvisioningErrorDetails: resource.Status.ProvisioningErrorDetails,
		},
	}
	*dbOutputResources = append(*dbOutputResources, dbr)
}

func (dp *deploymentProcessor) orderActions(actions map[string]ComponentAction) ([]ComponentAction, error) {
	unordered := []graph.DependencyItem{}
	for _, action := range actions {
		unordered = append(unordered, action)
	}

	dg, err := graph.ComputeDependencyGraph(unordered)
	if err != nil {
		return nil, err
	}

	items, err := dg.Order()
	if err != nil {
		return nil, err
	}

	ordered := []ComponentAction{}
	for _, item := range items {
		ordered = append(ordered, item.(ComponentAction))
	}

	return ordered, nil
}

func (dp *deploymentProcessor) DeleteDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus) error {
	logger := radlogger.GetLogger(ctx)

	logger.Info("Deleting deployment")
	errs := []error{}
	for _, wl := range d.Workloads {
		logger := logger.WithValues(radlogger.LogFieldComponentName, wl.ComponentName)
		logger.Info("Deleting workload")
		for _, resource := range wl.Resources {
			logger.WithValues(
				radlogger.LogFieldResourceType, resource.Type,
				radlogger.LogFieldResourceProperties, resource.Properties,
			).Info("Deleting resource")

			resourceType, err := dp.appmodel.LookupResource(resource.Type)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			err = resourceType.Handler().Delete(ctx, handlers.DeleteOptions{
				Application: appName,
				Component:   wl.ComponentName,
				Existing:    resource,
			})

			healthID := resource.Properties[healthcontract.HealthIDKey]
			dp.UnregisterForHealthChecks(ctx, healthID)

			if err != nil {
				errs = append(errs, fmt.Errorf("failed deleting resource %s of workload %v: %v", resource.Type, wl.ComponentName, err))
				continue
			}
		}
	}

	compositeErr := CompositeError{errs}
	if len(errs) > 0 {
		logger.Error(fmt.Errorf(compositeErr.Error()), fmt.Sprintf("Deletion of deployment completed with %d errors", len(errs)))
		return &compositeErr
	}

	logger.Info("Deployment deleted successfully")
	return nil
}

func (dp *deploymentProcessor) renderWorkload(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	ctx = radlogger.WrapLogContext(ctx,
		radlogger.LogFieldWorkLoadKind, w.Workload.Kind,
		radlogger.LogFieldWorkLoadName, w.Name)

	componentKind, err := dp.appmodel.LookupComponent(w.Workload.Kind)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	resources, err := componentKind.Renderer().Render(ctx, w)
	if err != nil {
		return resources, fmt.Errorf("could not render workload of kind %v: %v", w.Workload.Kind, err)
	}

	return resources, nil
}

func (dp *deploymentProcessor) processBindings(ctx context.Context, w workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties, bindingValues map[components.BindingKey]components.BindingState) error {
	componentKind, err := dp.appmodel.LookupComponent(w.Workload.Kind)
	if err != nil {
		return err
	}

	bindings, err := componentKind.Renderer().AllocateBindings(ctx, w, resources)
	if err != nil {
		return fmt.Errorf("could not allocate bindings for component %s of kind %v: %w", w.Name, w.Workload.Kind, err)
	}

	for name, state := range bindings {
		key := components.BindingKey{
			Component: w.Name,
			Binding:   name,
		}

		bindingValues[key] = state
	}

	// Validate that all user-specified bindings are present
	for name, binding := range w.Workload.Bindings {
		key := components.BindingKey{
			Component: w.Name,
			Binding:   name,
		}
		_, ok := bindingValues[key]
		if !ok {
			return fmt.Errorf(
				"the binding %s with kind %s of component %s is not supported by component kind %s",
				name,
				binding.Kind,
				w.Workload.Name,
				w.Workload.Kind)
		}
	}

	return nil
}

func (dp *deploymentProcessor) RegisterForHealthChecks(ctx context.Context, appID string, component db.Component) error {
	logger := radlogger.GetLogger(ctx)
	logger = logger.WithValues(
		radlogger.LogFieldComponentName, component.Name,
	)
	var errs []error
	for _, or := range component.Properties.Status.OutputResources {
		outputResourceInfo := healthcontract.ResourceDetails{
			ResourceID:     or.ResourceID,
			ResourceKind:   or.ResourceKind,
			ApplicationID:  appID,
			ComponentID:    component.Name,
			SubscriptionID: component.SubscriptionID,
			ResourceGroup:  component.ResourceGroup,
		}
		resourceType, err := dp.appmodel.LookupResource(or.ResourceKind)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		dp.registerOutputResourceForHealthChecks(ctx, outputResourceInfo, or.HealthID, resourceType.HealthHandler().GetHealthOptions(ctx))
		logger.WithValues(
			radlogger.LogFieldLocalID, or.LocalID,
			radlogger.LogFieldHealthID, or.HealthID).Info(fmt.Sprintf("Registered output resource with HealthID: %s for health checks", or.HealthID))
	}

	if len(errs) > 0 {
		return &CompositeError{Errors: errs}
	}

	return nil
}

func (dp *deploymentProcessor) registerOutputResourceForHealthChecks(ctx context.Context, healthInfo healthcontract.ResourceDetails, healthID string, options healthcontract.HealthCheckOptions) {
	if healthInfo.ResourceID == "" || healthInfo.ResourceKind == "" || healthID == "" {
		// TODO: Health status is not completely implemented for all resource kinds.
		// Adding this check for now to bypass this for unimplemented resources
		return
	}

	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldResourceID, healthInfo.ResourceID,
		radlogger.LogFieldHealthID, healthID,
		radlogger.LogFieldResourceType, healthInfo.ResourceID)

	logger.Info("Registering resource with the health service...")
	resourceInfo := healthcontract.ResourceInfo{
		HealthID:     healthID,
		ResourceID:   healthInfo.ResourceID,
		ResourceKind: healthInfo.ResourceKind,
	}
	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: resourceInfo,
		Options:      options,
	}
	dp.health.ResourceRegistrationWithHealthChannel <- msg
}

func (dp *deploymentProcessor) UnregisterForHealthChecks(ctx context.Context, healthID string) {
	logger := radlogger.GetLogger(ctx)
	logger.Info("Unregistering resource with the health service...")
	msg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionUnregister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID: healthID,
		},
	}
	dp.health.ResourceRegistrationWithHealthChannel <- msg
}
