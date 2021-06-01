// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/radius/pkg/algorithm/graph"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/curp/revision"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbsqlv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/inboundroute"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
	"github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

//go:generate mockgen -destination=../../../mocks/mock_deployment_processor.go -package=mocks github.com/Azure/radius/pkg/curp/deployment DeploymentProcessor

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
	dispatcher workloads.WorkloadDispatcher
	rm         resourceManager
	k8s        client.Client
}

type resourceManager struct {
	handlers map[string]handlers.ResourceHandler
}

// NewDeploymentProcessor initializes a deployment processor.
func NewDeploymentProcessor(arm armauth.ArmConfig, k8s client.Client) DeploymentProcessor {
	d := workloads.Dispatcher{
		Renderers: map[string]workloads.WorkloadRenderer{
			daprstatestorev1alpha1.Kind:  &daprstatestorev1alpha1.Renderer{},
			daprpubsubv1alpha1.Kind:      &daprpubsubv1alpha1.Renderer{},
			cosmosdbmongov1alpha1.Kind:   &cosmosdbmongov1alpha1.Renderer{Arm: arm},
			cosmosdbsqlv1alpha1.Kind:     &cosmosdbsqlv1alpha1.Renderer{Arm: arm},
			containerv1alpha1.Kind:       &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: arm}}},
			servicebusqueuev1alpha1.Kind: &servicebusqueuev1alpha1.Renderer{Arm: arm},
			keyvaultv1alpha1.Kind:        &keyvaultv1alpha1.Renderer{Arm: arm},
		},
	}

	rm := resourceManager{
		handlers: map[string]handlers.ResourceHandler{
			workloads.ResourceKindKubernetes:                     handlers.NewKubernetesHandler(k8s),
			workloads.ResourceKindDaprStateStoreAzureStorage:     handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
			workloads.ResourceKindDaprStateStoreSQLServer:        handlers.NewDaprStateStoreSQLServerHandler(arm, k8s),
			workloads.ResourceKindDaprPubSubTopicAzureServiceBus: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
			workloads.ResourceKindAzureCosmosDBMongo:             handlers.NewAzureCosmosDBMongoHandler(arm),
			workloads.ResourceKindAzureCosmosDBSQL:               handlers.NewAzureCosmosDBSQLHandler(arm),
			workloads.ResourceKindAzureServiceBusQueue:           handlers.NewAzureServiceBusQueueHandler(arm),
			workloads.ResourceKindAzureKeyVault:                  handlers.NewAzureKeyVaultHandler(arm),
			workloads.ResourceKindAzurePodIdentity:               handlers.NewAzurePodIdentityHandler(arm),
		},
	}

	return &deploymentProcessor{d, rm, k8s}
}

func (dp *deploymentProcessor) UpdateDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus, actions map[string]ComponentAction) error {
	// First create a namespace for our stuff to live
	//
	// TODO: right now we have the assumption that all of the k8s resources will be generated
	// in the same namespace as the application.
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": appName,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
		},
	}
	err := dp.k8s.Patch(ctx, ns, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		// we consider this fatal - without a namespace we won't be able to apply anything else
		return fmt.Errorf("error applying namespace: %w", err)
	}

	// TODO - any sort of rollback - we'll leave things in a partially-created state
	// for now if we encounter a failure at any point.
	//
	// TODO - we don't handle the case where resources disappear without the component being
	// deleted. All of the resources we support so far are 1-1 with the components.
	errs := []error{}

	ordered, err := dp.orderActions(actions)
	if err != nil {
		return err
	}

	log.Printf("actions in order:")
	for i, action := range ordered {
		log.Printf("%v - %s", i, action.ComponentName)
	}

	bindingValues := map[components.BindingKey]components.BindingState{}

	// Process each action and update the deployment status as we go ...
	for _, action := range ordered {
		log.Printf("executing action %s for component %s", action.Operation, action.ComponentName)

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

			resources, err := dp.renderWorkload(ctx, inst)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			var existingStatus *db.DeploymentWorkload
			for _, existing := range d.Workloads {
				if existing.ComponentName == action.ComponentName {
					existingStatus = &existing
					break
				}
			}

			for _, resource := range resources {
				var existingResource *db.DeploymentResource
				if existingStatus != nil {
					for _, existing := range existingStatus.Resources {
						if existing.LocalID == resource.LocalID {
							existingResource = &existing
							break
						}
					}
				}

				h, ok := dp.rm.handlers[resource.Type]
				if !ok {
					errs = append(errs, fmt.Errorf("cannot find handler for resource type %s", resource.Type))
					continue
				}

				properties, err := h.Put(ctx, handlers.PutOptions{
					Application: appName,
					Component:   action.ComponentName,
					Resource:    resource,
					Existing:    existingResource,
				})
				if err != nil {
					errs = append(errs, fmt.Errorf("error applying workload resource %v %v: %w", properties, action.ComponentName, err))
					continue
				}

				dr := db.DeploymentResource{
					LocalID:    resource.LocalID,
					Type:       resource.Type,
					Properties: properties,
				}
				dw.Resources = append(dw.Resources, dr)
			}

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

			log.Printf("successfully applied workload %v %v", action.Component.Kind, action.ComponentName)

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
				h, ok := dp.rm.handlers[resource.Type]
				if !ok {
					errs = append(errs, fmt.Errorf("cannot find handler for resource type %s", resource.Type))
					continue
				}

				err = h.Delete(ctx, handlers.DeleteOptions{
					Application: appName,
					Component:   action.ComponentName,
					Existing:    resource,
				})
				if err != nil {
					errs = append(errs, fmt.Errorf("error deleting workload resource %v %v: %w", resource.Properties, action.ComponentName, err))
					continue
				}
			}

			log.Printf("successfully deleted workload %v", action.ComponentName)
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
	log.Printf("Deleting deployment %v", name)
	errs := []error{}
	for _, wl := range d.Workloads {
		log.Printf("Deleting workload %v", wl.ComponentName)
		for _, resource := range wl.Resources {
			log.Printf("Deleting resource %v %v", resource.Type, resource.Properties)

			h, ok := dp.rm.handlers[resource.Type]
			if !ok {
				errs = append(errs, fmt.Errorf("cannot find handler for resource type %s", resource.Type))
				continue
			}

			err := h.Delete(ctx, handlers.DeleteOptions{
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

	log.Printf("Deletion of deployment %v completed with %d errors", name, len(errs))
	if len(errs) > 0 {
		return &CompositeError{errs}
	}

	return nil
}

func (dp *deploymentProcessor) renderWorkload(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	r, err := dp.dispatcher.Lookup(w.Workload.Kind)
	if err != nil {
		return []workloads.WorkloadResource{}, fmt.Errorf("could not render workload of kind %v: %v", w.Workload.Kind, err)
	}

	resources, err := r.Render(ctx, w)
	if err != nil {
		return []workloads.WorkloadResource{}, fmt.Errorf("could not render workload of kind %v: %v", w.Workload.Kind, err)
	}

	return resources, nil
}

func (dp *deploymentProcessor) processBindings(ctx context.Context, w workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties, bindingValues map[components.BindingKey]components.BindingState) error {

	renderer, err := dp.dispatcher.Lookup(w.Workload.Kind)
	if err != nil {
		return fmt.Errorf("could not find renderer for workload of kind %v: %w", w.Workload.Kind, err)
	}

	bindings, err := renderer.AllocateBindings(ctx, w, resources)
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
