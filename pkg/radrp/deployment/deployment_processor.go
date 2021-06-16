// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/algorithm/graph"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/revision"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
)

// DeploymentOperation represents an operation performed on a workload.
type DeploymentOperation string

const (
	// None represents a workload that's unchanged in a deployment.
	None DeploymentOperation = "none"

	// DeleteWorkload represents deleting a workload from deployment.
	DeleteWorkload DeploymentOperation = "delete"

	// CreateWorkload represents creating a workload in deployment.
	CreateWorkload DeploymentOperation = "create"

	// UpdateWorkload represents updating a workload in deployment.
	UpdateWorkload DeploymentOperation = "update"
)

// ComponentAction represents a set of deployment actions to take for a component instance.
type ComponentAction struct {
	ApplicationName string
	ComponentName   string
	Operation       DeploymentOperation

	NewRevision revision.Revision
	OldRevision revision.Revision

	// Will be `nil` for a delete
	Definition *db.Component
	// Will be `nil` for a delete
	Component *components.GenericComponent
}

// DependencyItem implementation
func (action ComponentAction) Key() string {
	return action.ComponentName
}

func (action ComponentAction) GetDependencies() []string {
	dependencies := []string{}
	for _, dependency := range action.Component.Uses {
		if dependency.Binding.Kind == components.KindStatic {
			continue
		}

		expr := dependency.Binding.Value.(*components.ComponentBindingValue)
		dependencies = append(dependencies, expr.Component)
	}

	return dependencies
}

//go:generate mockgen -destination=../../../mocks/mock_deployment_processor.go -package=mocks github.com/Azure/radius/pkg/radrp/deployment DeploymentProcessor

// DeploymentProcessor implements functionality for updating and deleting deployments.
type DeploymentProcessor interface {
	UpdateDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus, actions map[string]ComponentAction) error
	DeleteDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus) error
}

// CompositeError represents an error containing multiple failures.
type CompositeError struct {
	Errors []error
}

func (ce *CompositeError) Error() string {
	if len(ce.Errors) == 1 {
		return ce.Errors[0].Error()
	}

	ss := make([]string, len(ce.Errors))
	for i, e := range ce.Errors {
		ss[i] = e.Error()
	}
	return "multiple errors: " + strings.Join(ss, ",")
}

type deploymentProcessor struct {
	appmodel model.ApplicationModel
	logger   logr.Logger
}

// NewDeploymentProcessor initializes a deployment processor.
func NewDeploymentProcessor(appmodel model.ApplicationModel, logger logr.Logger) DeploymentProcessor {
	return &deploymentProcessor{
		appmodel: appmodel,
		logger:   logger,
	}
}

func (dp *deploymentProcessor) UpdateDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus, actions map[string]ComponentAction) error {
	// TODO - any sort of rollback - we'll leave things in a partially-created state
	// for now if we encounter a failure at any point.
	//
	// TODO - we don't handle the case where resources disappear without the component being
	// deleted. All of the resources we support so far are 1-1 with the components.
	errs := []error{}
	logger := dp.logger.WithValues(
		radlogger.LogFieldAppName, appName,
	)

	ordered, err := dp.orderActions(actions)
	if err != nil {
		return err
	}

	logger.Info("actions in order:")
	for i, action := range ordered {
		logger.Info(
			fmt.Sprintf("%v", i),
			radlogger.LogFieldComponentName, action.ComponentName,
		)
	}

	bindingValues := map[components.BindingKey]components.BindingState{}

	// Process each action and update the deployment status as we go ...
	for _, action := range ordered {
		logger = logger.WithValues(
			radlogger.LogFieldAction, action.Operation,
			radlogger.LogFieldComponentName, action.ComponentName,
		)
		logger.Info("executing")

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
			for _, status := range d.Workloads {
				if status.ComponentName != action.Component.Name {
					continue
				}

				for _, resource := range status.Resources {
					wr := workloads.WorkloadResourceProperties{
						Type:       resource.Type,
						Properties: resource.Properties,
					}
					resources = append(resources, wr)
				}
			}

			inst := workloads.InstantiatedWorkload{
				Application:   appName,
				Name:          action.ComponentName,
				Workload:      *action.Component,
				BindingValues: bindingValues,
			}

			err = dp.processBindings(ctx, inst, resources, bindingValues)
			if err != nil {
				errs = append(errs, fmt.Errorf("error applying bindings for component %v : %w", action.ComponentName, err))
				continue
			}

		case CreateWorkload, UpdateWorkload:
			// For an update, just blow away the existing workload record
			dw := db.DeploymentWorkload{
				ComponentName: action.ComponentName,
				Kind:          action.Definition.Kind,
			}

			inst := workloads.InstantiatedWorkload{
				Application:   appName,
				Name:          action.ComponentName,
				Workload:      *action.Component,
				BindingValues: bindingValues,
			}

			outputResources, err := dp.renderWorkload(ctx, inst)
			if err != nil {
				errs = append(errs, err)
				dbOutputResources := []db.OutputResource{}
				for _, resource := range outputResources {
					// Even if the operation fails, return the output resources created so far
					// TODO: This is temporary. Once there are no resources actually deployed during render phase,
					// we no longer need to track the output resources on error
					addDBOutputResource(resource, &dbOutputResources)
				}
				action.Definition.Properties.OutputResources = dbOutputResources
				continue
			}

			var existingStatus *db.DeploymentWorkload
			for _, existing := range d.Workloads {
				if existing.ComponentName == action.ComponentName {
					existingStatus = &existing
					break
				}
			}

			dbOutputResources := []db.OutputResource{}
			for _, resource := range outputResources {
				var existingResource *db.DeploymentResource
				if existingStatus != nil {
					for _, existing := range existingStatus.Resources {
						if existing.LocalID == resource.LocalID {
							existingResource = &existing
							break
						}
					}
				}

				resourceType, err := dp.appmodel.LookupResource(resource.ResourceKind)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				properties, err := resourceType.Handler().Put(ctx, handlers.PutOptions{
					Application: appName,
					Component:   action.ComponentName,
					Resource:    resource,
					Existing:    existingResource,
				})
				// Record the output resources created so far
				addDBOutputResource(resource, &dbOutputResources)

				if err != nil {
					errs = append(errs, fmt.Errorf("error applying workload resource %v %v: %w", properties, action.ComponentName, err))
					continue
				}

				dr := db.DeploymentResource{
					LocalID:    resource.LocalID,
					Type:       resource.ResourceKind,
					Properties: properties,
				}
				dw.Resources = append(dw.Resources, dr)
			}

			// Add the output resources to the DB component definition
			action.Definition.Properties.OutputResources = dbOutputResources

			wrps := []workloads.WorkloadResourceProperties{}
			for _, resource := range dw.Resources {
				wr := workloads.WorkloadResourceProperties{
					Type:       resource.Type,
					Properties: resource.Properties,
				}
				wrps = append(wrps, wr)
			}

			// Populate data for the bindings that this component provides
			err = dp.processBindings(ctx, inst, wrps, bindingValues)
			if err != nil {
				errs = append(errs, fmt.Errorf("error applying workload bindings %v: %w", action.ComponentName, err))
				continue
			}

			updated := false
			for i, existing := range d.Workloads {
				if existing.ComponentName == dw.ComponentName {
					d.Workloads[i] = dw
					updated = true
					break
				}
			}

			if !updated {
				d.Workloads = append(d.Workloads, dw)
			}

			logger.Info(
				"successfully applied workload",
				radlogger.LogFieldComponentKind, action.Component.Kind,
			)

		case DeleteWorkload:
			// Remove the deployment record
			var match db.DeploymentWorkload
			for i, existing := range d.Workloads {
				if existing.ComponentName == action.ComponentName {
					match = existing
					d.Workloads = append(d.Workloads[:i], d.Workloads[i+1:]...)
					break
				}
			}

			if match.ComponentName == "" {
				errs = append(errs, fmt.Errorf("cannot find deployment record for %v", action.ComponentName))
				continue
			}

			for _, resource := range match.Resources {
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
				if err != nil {
					errs = append(errs, fmt.Errorf("error deleting workload resource %v %v: %w", resource.Properties, action.ComponentName, err))
					continue
				}
			}

			logger.Info("successfully deleted workload")
		}
	}

	names := map[string]bool{}
	for _, dw := range d.Workloads {
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

func addDBOutputResource(resource workloads.OutputResource, dbOutputResources *[]db.OutputResource) {
	// Save the output resource to DB
	dbr := db.OutputResource{
		Managed:            resource.Managed,
		LocalID:            resource.LocalID,
		ResourceKind:       resource.ResourceKind,
		OutputResourceInfo: resource.OutputResourceInfo,
		OutputResourceType: resource.OutputResourceType,
		Resource:           resource.Resource,
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
	logger := dp.logger.WithValues(
		radlogger.LogFieldDeploymentName, name,
	)

	logger.Info(
		"Deleting deployment",
	)
	errs := []error{}
	for _, wl := range d.Workloads {
		logger := logger.WithValues(
			radlogger.LogFieldComponentName, wl.ComponentName,
		)
		logger.Info("Deleting workload")
		for _, resource := range wl.Resources {
			logger.Info(
				"Deleting resource",
				radlogger.LogFieldResourceType, resource.Type,
				radlogger.LogFieldResourceProperties, resource.Properties,
			)

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
			if err != nil {
				errs = append(errs, fmt.Errorf("failed deleting resource %s of workload %v: %v", resource.Type, wl.ComponentName, err))
				continue
			}
		}
	}

	dp.logger.Info(
		fmt.Sprintf("Deletion of deployment completed with %d errors", len(errs)),
		radlogger.LogFieldDeploymentName, name,
	)
	if len(errs) > 0 {
		return &CompositeError{errs}
	}

	return nil
}

func (dp *deploymentProcessor) renderWorkload(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	componentKind, err := dp.appmodel.LookupComponent(w.Workload.Kind)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

	resources, err := componentKind.Renderer().Render(ctx, w)
	log.Printf("Created output resources for workload: %s\n", w.Name)
	for _, o := range resources {
		log.Printf("LocalID: %s, output resource type: %s\n", o.LocalID, o.OutputResourceType)
	}
	if err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
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
