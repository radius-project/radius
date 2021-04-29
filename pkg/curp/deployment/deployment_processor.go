// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/rad/namegenerator"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdocumentdbv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/ingress"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
	"github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1"
	"github.com/gofrs/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	Definition      *db.ComponentRevision
	Instantiation   *db.DeploymentComponent
	Provides        map[string]ComponentService
	ServiceBindings map[string]ServiceBinding

	// Will be `nil` for a delete
	Component             *components.GenericComponent
	PreviousDefinition    *db.ComponentRevision
	PreviousInstanitation *db.DeploymentComponent
}

// ComponentService represents a service provided by this component
type ComponentService struct {
	Name     string
	Kind     string
	Provider string
}

// ServiceBinding represents the binding between a component that provides a service, and those that consume it.
type ServiceBinding struct {
	Name     string
	Kind     string
	Provider string
}

//go:generate mockgen -destination=../../../mocks/mock_deployment_processor.go -package=mocks github.com/Azure/radius/pkg/curp/deployment DeploymentProcessor

// DeploymentProcessor implements functionality for updating and deleting deployments.
type DeploymentProcessor interface {
	UpdateDeployment(ctx context.Context, appName string, name string, d *db.DeploymentStatus, actions map[string]ComponentAction) error
	DeleteDeployment(ctx context.Context, name string, d *db.DeploymentStatus) error
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
	handlers map[string]resourceHandler
}

type resourceHandler interface {
	GetProperties(resource workloads.WorkloadResource) (map[string]string, error)
	Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error)
	Delete(ctx context.Context, properties map[string]string) error
}

type kubernetesHandler struct {
	k8s client.Client
}

type storageStateStoreHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

type servicebusPubSubTopicHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

type cosmosDocumentDbHandler struct {
	arm armauth.ArmConfig
}

type serviceBusQueueHandler struct {
	arm armauth.ArmConfig
}

type keyVaultHandler struct {
	arm armauth.ArmConfig
}

type podIdentityHandler struct {
	arm armauth.ArmConfig
}

// NewDeploymentProcessor initializes a deployment processor.
func NewDeploymentProcessor(arm armauth.ArmConfig, k8s client.Client) DeploymentProcessor {
	d := workloads.Dispatcher{
		Renderers: map[string]workloads.WorkloadRenderer{
			daprstatestorev1alpha1.Kind:   &daprstatestorev1alpha1.Renderer{},
			daprpubsubv1alpha1.Kind:       &daprpubsubv1alpha1.Renderer{},
			cosmosdocumentdbv1alpha1.Kind: &cosmosdocumentdbv1alpha1.Renderer{Arm: arm},
			containerv1alpha1.Kind:        &ingress.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: arm}}},
			servicebusqueuev1alpha1.Kind:  &servicebusqueuev1alpha1.Renderer{Arm: arm},
			keyvaultv1alpha1.Kind:         &keyvaultv1alpha1.Renderer{Arm: arm},
		},
	}

	rm := resourceManager{
		handlers: map[string]resourceHandler{
			"kubernetes":                       &kubernetesHandler{k8s},
			"dapr.statestore.azurestorage":     &storageStateStoreHandler{arm, k8s},
			"dapr.pubsubtopic.azureservicebus": &servicebusPubSubTopicHandler{arm, k8s},
			"azure.cosmos.documentdb":          &cosmosDocumentDbHandler{arm},
			"azure.servicebus.queue":           &serviceBusQueueHandler{arm},
			"azure.keyvault":                   &keyVaultHandler{arm},
			"azure.aadpodidentity":             &podIdentityHandler{arm},
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

	// Process each action and update the deployment status as we go ...
	for _, action := range ordered {
		log.Printf("executing action %s for component %s", action.Operation, action.ComponentName)

		// while we do bookkeeping, also update the deployment record
		switch action.Operation {

		case None:
			// Don't update resources or services - we should already have them from the DB

		case CreateWorkload, UpdateWorkload:
			// For an update, just blow away the existing workload record
			dw := db.DeploymentWorkload{
				ComponentName: action.ComponentName,
				Kind:          action.Definition.Kind,
			}

			// Retrieve the services this component depends on - they should already be populated
			// either from a previous deployment or from rendering during this one.
			values := map[string]map[string]interface{}{}
			for _, binding := range action.ServiceBindings {
				s, ok := d.Services[binding.Name]
				if !ok {
					errs = append(errs, fmt.Errorf("cannot find service %v : %v - provider should be %v", binding.Name, binding.Kind, binding.Provider))
					continue
				}

				if s.Kind != binding.Kind {
					errs = append(errs, fmt.Errorf("service %v : %v - is not of expected kind %v - provider should be %v", s.Name, s.Kind, binding.Kind, binding.Provider))
					continue
				}

				values[binding.Name] = s.Properties
			}

			inst := workloads.InstantiatedWorkload{
				Application:   appName,
				Name:          action.ComponentName,
				Workload:      *action.Component,
				ServiceValues: values,
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

				properties, err := h.Put(ctx, resource, existingResource)
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

			// Fetch all the services this component provides
			services, err := dp.renderServices(ctx, inst, dw.Resources, action.Provides)
			if err != nil {
				errs = append(errs, fmt.Errorf("error applying workload services %v: %w", action.ComponentName, err))
				continue
			}

			// Track services centrally
			for _, s := range services {
				d.Services[s.Name] = s
			}

			// Remove services this component provides, they are not eligible anymore
			removeUnreachableServices(*d, action.ComponentName, action.Provides)

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

			// Remove services this component provides, they are not eligible anymore
			removeUnreachableServices(*d, action.ComponentName, action.Provides)

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

				err = h.Delete(ctx, resource.Properties)
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

func removeUnreachableServices(d db.DeploymentStatus, componentName string, providers map[string]ComponentService) {
	remove := map[string]bool{}
	for k, s := range d.Services {
		// looking for services that were defined by this component but are no longer
		_, provides := providers[k]
		if s.Provider == componentName && !provides {
			remove[k] = true
		}
	}

	for k := range remove {
		delete(d.Services, k)
	}
}

func (dp *deploymentProcessor) orderActions(actions map[string]ComponentAction) ([]ComponentAction, error) {
	// TODO: reimplement this as an in-place sort on a single slice rather than this O(N^3) monstrosity
	done := map[string]bool{}
	ordered := []ComponentAction{}

	for {
		if len(done) == len(actions) {
			// all actions ordered
			return ordered, nil
		}

		progress := false
		for name, action := range actions {
			if _, ok := done[name]; ok {
				// already ordered
				continue
			}

			ready := true
			for _, binding := range action.ServiceBindings {
				if _, ok := done[binding.Provider]; !ok {
					// this component has an outstanding dependency
					ready = false
					break
				}
			}

			if ready {
				ordered = append(ordered, action)
				done[name] = true
				progress = true
				break
			}

			// else, try the next component
		}

		if !progress {
			return []ComponentAction{}, errors.New("circular dependency detected")
		}
	}
}

func (dp *deploymentProcessor) DeleteDeployment(ctx context.Context, name string, d *db.DeploymentStatus) error {
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

			err := h.Delete(ctx, resource.Properties)
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

func (dp *deploymentProcessor) renderServices(ctx context.Context, w workloads.InstantiatedWorkload, dr []db.DeploymentResource, services map[string]ComponentService) ([]db.DeploymentService, error) {
	r, err := dp.dispatcher.Lookup(w.Workload.Kind)
	if err != nil {
		return []db.DeploymentService{}, fmt.Errorf("could not render workload of kind %v: %v", w.Workload.Kind, err)
	}

	resources := []workloads.WorkloadResourceProperties{}
	for _, r := range dr {
		resources = append(resources, workloads.WorkloadResourceProperties{
			Type:       r.Type,
			Properties: r.Properties,
		})
	}

	results := []db.DeploymentService{}
	for _, s := range services {
		service := workloads.WorkloadService{Name: s.Name, Kind: s.Kind}
		values, err := r.Allocate(ctx, w, resources, service)
		if err != nil {
			return []db.DeploymentService{}, fmt.Errorf("could not allocate service of kind %v: %v", s.Kind, err)
		}

		results = append(results, db.DeploymentService{Name: s.Name, Kind: s.Kind, Provider: w.Workload.Kind, Properties: values})
	}

	return results, nil
}

func (kh *kubernetesHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	p := map[string]string{
		"kind":       item.GetKind(),
		"apiVersion": item.GetAPIVersion(),
		"namespace":  item.GetNamespace(),
		"name":       item.GetName(),
	}
	return p, nil
}

func (kh *kubernetesHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	// can ignore existing resource

	p := map[string]string{
		"kind":       item.GetKind(),
		"apiVersion": item.GetAPIVersion(),
		"namespace":  item.GetNamespace(),
		"name":       item.GetName(),
	}

	err = kh.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return p, err
}

func (kh *kubernetesHandler) Delete(ctx context.Context, properties map[string]string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
		},
	}

	return client.IgnoreNotFound(kh.k8s.Delete(ctx, &item))
}

func convertToUnstructured(resource workloads.WorkloadResource) (unstructured.Unstructured, error) {
	if resource.Type != "kubernetes" {
		return unstructured.Unstructured{}, errors.New("wrong resource type")
	}

	obj, ok := resource.Resource.(runtime.Object)
	if !ok {
		return unstructured.Unstructured{}, errors.New("inner type was not a runtime.Object")
	}

	c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource.Resource)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("could not convert object %v to unstructured: %w", obj.GetObjectKind(), err)
	}

	return unstructured.Unstructured{Object: c}, nil
}

func mergeProperties(properties map[string]string, existing *db.DeploymentResource) {
	if existing == nil {
		return
	}

	for k, v := range existing.Properties {
		_, ok := properties[k]
		if !ok {
			properties[k] = v
		}
	}
}

func (sssh *storageStateStoreHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	if resource.Type != "dapr.statestore.azurestorage" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	return properties, nil
}

func (sssh *storageStateStoreHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	if resource.Type != "dapr.statestore.azurestorage" {
		return nil, errors.New("wrong resource type")
	}

	sc := storage.NewAccountsClient(sssh.arm.SubscriptionID)
	sc.Authorizer = sssh.arm.Auth

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	mergeProperties(properties, existing)

	name, ok := properties["storageaccountname"]
	if !ok {
		// names are kinda finicky here - they have to be unique across azure.
		base := properties["name"]
		name = ""

		for i := 0; i < 10; i++ {
			// 3-24 characters - all alphanumeric
			uid, err := uuid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to generate storage account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			result, err := sc.CheckNameAvailability(ctx, storage.AccountCheckNameAvailabilityParameters{
				Name: to.StringPtr(name),
				Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to query storage account name: %w", err)
			}

			if result.NameAvailable != nil && *result.NameAvailable {
				properties["storageaccountname"] = name
				break
			}

			log.Printf("storage account name generation failed: %v %v", result.Reason, result.Message)
		}
	}

	if name == "" {
		return nil, fmt.Errorf("failed to find a storage name")
	}

	// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
	// in something to do with data locality.
	rgc := resources.NewGroupsClient(sssh.arm.SubscriptionID)
	rgc.Authorizer = sssh.arm.Auth

	g, err := rgc.Get(ctx, sssh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	future, err := sc.Create(ctx, sssh.arm.ResourceGroup, name, storage.AccountCreateParameters{
		Location: g.Location,
		Kind:     storage.StorageV2,
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
		},
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	account, err := future.Result(sc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	// store storage account so we can delete later
	properties["storageaccountid"] = *account.ID

	keys, err := sc.ListKeys(ctx, sssh.arm.ResourceGroup, name, "")
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	// Since we're doing this programmatically, let's make sure we can find a key with write access.
	if keys.Keys == nil || len(*keys.Keys) == 0 {
		return nil, fmt.Errorf("listkeys returned an empty or nil list of keys")
	}

	// Don't rely on the order the keys are in, we need Full access
	var key *storage.AccountKey
	for _, k := range *keys.Keys {
		if strings.EqualFold(string(k.Permissions), string(storage.Full)) {
			key = &k
			break
		}
	}

	if key == nil {
		return nil, fmt.Errorf("listkeys contained keys, but none of them have full access")
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
			"spec": map[string]interface{}{
				"type":    "state.azure.tablestorage",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "accountName",
						"value": name,
					},
					map[string]interface{}{
						"name":  "accountKey",
						"value": *key.Value,
					},
					map[string]interface{}{
						"name":  "tableName",
						"value": "dapr",
					},
				},
			},
		},
	}

	err = sssh.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (sssh *storageStateStoreHandler) Delete(ctx context.Context, properties map[string]string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
		},
	}

	err := client.IgnoreNotFound(sssh.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	sc := storage.NewAccountsClient(sssh.arm.SubscriptionID)
	sc.Authorizer = sssh.arm.Auth

	// TODO: gross workaround - sorry everyone :(
	if properties["storageaccountname"] == "" {
		return nil
	}

	_, err = sc.Delete(ctx, sssh.arm.ResourceGroup, properties["storageaccountname"])
	if err != nil {
		return err
	}

	return nil
}

func (pssb *servicebusPubSubTopicHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	if resource.Type != "dapr.pubsubtopic.azureservicebus" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	return properties, nil
}

func (pssb *servicebusPubSubTopicHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	if resource.Type != "dapr.pubsubtopic.azureservicebus" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	mergeProperties(properties, existing)

	sbc := servicebus.NewNamespacesClient(pssb.arm.SubscriptionID)
	sbc.Authorizer = pssb.arm.Auth

	// Check if a service bus namespace exists in the resource group
	sbItr, err := sbc.ListByResourceGroupComplete(ctx, pssb.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("Failed to list service bus namespaces: %w", err)
	}

	var sbNamespace servicebus.SBNamespace

	// Azure Service Bus needs StandardTier or higher SKU to support topics
	if sbItr.NotDone() && sbItr.Value().Sku.Tier != servicebus.SkuTierBasic {
		// A service bus namespace already exists
		sbNamespace = sbItr.Value()
	} else {
		// Generate a random namespace name
		namespaceName := namegenerator.GenerateName("radius-ns")

		// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
		// in something to do with data locality.
		rgc := resources.NewGroupsClient(pssb.arm.SubscriptionID)
		rgc.Authorizer = pssb.arm.Auth

		g, err := rgc.Get(ctx, pssb.arm.ResourceGroup)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		sbNamespaceFuture, err := sbc.CreateOrUpdate(ctx, pssb.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
			Sku: &servicebus.SBSku{
				Name:     servicebus.Standard,
				Tier:     servicebus.SkuTierStandard,
				Capacity: to.Int32Ptr(1),
			},
			Location: g.Location,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		sbNamespace, err = sbNamespaceFuture.Result(sbc)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}
	}

	properties["servicebusnamespace"] = *sbNamespace.Name
	properties["servicebusid"] = *sbNamespace.ID

	topicName, ok := properties["servicebustopic"]
	if !ok {
		return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
	}
	tc := servicebus.NewTopicsClient(pssb.arm.SubscriptionID)
	tc.Authorizer = pssb.arm.Auth

	sbTopic, err := tc.CreateOrUpdate(ctx, pssb.arm.ResourceGroup, *sbNamespace.Name, topicName, servicebus.SBTopic{
		Name: to.StringPtr(topicName),
		SBTopicProperties: &servicebus.SBTopicProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT servicebus topic: %w", err)
	}

	// store db so we can delete later
	properties["topicName"] = *sbTopic.Name

	accessKeys, err := sbc.ListKeys(ctx, pssb.arm.ResourceGroup, *sbNamespace.Name, "RootManageSharedAccessKey")

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil && accessKeys.SecondaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	cs := accessKeys.PrimaryConnectionString

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
			"spec": map[string]interface{}{
				"type":    "pubsub.azure.servicebus",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "connectionString",
						"value": cs,
					},
				},
			},
		},
	}

	err = pssb.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (pssb *servicebusPubSubTopicHandler) Delete(ctx context.Context, properties map[string]string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
		},
	}

	err := client.IgnoreNotFound(pssb.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	namespaceName := properties["servicebusnamespace"]
	topicName := properties["servicebustopic"]

	tc := servicebus.NewTopicsClient(pssb.arm.SubscriptionID)
	tc.Authorizer = pssb.arm.Auth

	_, err = tc.Delete(ctx, pssb.arm.ResourceGroup, namespaceName, topicName)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	tItr, err := tc.ListByNamespaceComplete(ctx, pssb.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	// Delete service bus topic only marks the topic for deletion but does not actually delete it. Hence the additional check...
	// https://docs.microsoft.com/en-us/rest/api/servicebus/delete-topic
	if tItr.NotDone() && tItr.Value().Name != &topicName {
		// There are other topics in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return nil
	}

	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := servicebus.NewNamespacesClient(pssb.arm.SubscriptionID)
	sbc.Authorizer = pssb.arm.Auth

	sbNamespaceFuture, err := sbc.Delete(ctx, pssb.arm.ResourceGroup, namespaceName)

	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	_, err = sbNamespaceFuture.Result(sbc)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	return nil
}

func (cddh *cosmosDocumentDbHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	if resource.Type != "azure.cosmos.documentdb" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	return properties, nil
}

func (cddh *cosmosDocumentDbHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	if resource.Type != "azure.cosmos.documentdb" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	mergeProperties(properties, existing)

	dac := documentdb.NewDatabaseAccountsClient(cddh.arm.SubscriptionID)
	dac.Authorizer = cddh.arm.Auth

	name, ok := properties["cosmosaccountname"]
	if !ok {
		// names are kinda finicky here - they have to be unique across azure.
		base := properties["name"] + "-"
		name = ""

		for i := 0; i < 10; i++ {
			// 3-24 characters - all alphanumeric and '-'
			uid, err := uuid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to generate storage account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			result, err := dac.CheckNameExists(ctx, name)
			if err != nil {
				return nil, fmt.Errorf("failed to query cosmos account name: %w", err)
			}

			if result.StatusCode == 404 {
				properties["cosmosaccountname"] = name
				break
			}

			log.Printf("cosmos account name generation failed")
		}
	}

	// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
	// in something to do with data locality.
	rgc := resources.NewGroupsClient(cddh.arm.SubscriptionID)
	rgc.Authorizer = cddh.arm.Auth

	g, err := rgc.Get(ctx, cddh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	accountFuture, err := dac.CreateOrUpdate(ctx, cddh.arm.ResourceGroup, name, documentdb.DatabaseAccountCreateUpdateParameters{
		Kind:     documentdb.MongoDB,
		Location: g.Location,
		DatabaseAccountCreateUpdateProperties: &documentdb.DatabaseAccountCreateUpdateProperties{
			DatabaseAccountOfferType: to.StringPtr("Standard"),
			Locations: &[]documentdb.Location{
				{
					LocationName: g.Location,
				},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, dac.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	account, err := accountFuture.Result(dac)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	// store account so we can delete later
	properties["cosmosaccountid"] = *account.ID

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	dbfuture, err := mrc.CreateUpdateMongoDBDatabase(ctx, cddh.arm.ResourceGroup, *account.Name, properties["name"], documentdb.MongoDBDatabaseCreateUpdateParameters{
		MongoDBDatabaseCreateUpdateProperties: &documentdb.MongoDBDatabaseCreateUpdateProperties{
			Resource: &documentdb.MongoDBDatabaseResource{
				ID: to.StringPtr(properties["name"]),
			},
			Options: &documentdb.CreateUpdateOptions{
				AutoscaleSettings: &documentdb.AutoscaleSettings{
					MaxThroughput: to.Int32Ptr(4000),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	db, err := dbfuture.Result(mrc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	// store db so we can delete later
	properties["databasename"] = *db.Name

	return properties, nil
}

func (cddh *cosmosDocumentDbHandler) Delete(ctx context.Context, properties map[string]string) error {
	accountname := properties["cosmosaccountname"]
	dbname := properties["databasename"]

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	dbfuture, err := mrc.DeleteMongoDBDatabase(ctx, cddh.arm.ResourceGroup, accountname, dbname)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	_, err = dbfuture.Result(mrc)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	dac := documentdb.NewDatabaseAccountsClient(cddh.arm.SubscriptionID)
	dac.Authorizer = cddh.arm.Auth

	accountFuture, err := dac.Delete(ctx, cddh.arm.ResourceGroup, accountname)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, dac.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	_, err = accountFuture.Result(dac)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	return nil
}

func (sbh *serviceBusQueueHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	if resource.Type != "azure.servicebus.queue" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	return properties, nil
}

func (sbh *serviceBusQueueHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	if resource.Type != "azure.servicebus.queue" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	mergeProperties(properties, existing)

	sbc := servicebus.NewNamespacesClient(sbh.arm.SubscriptionID)
	sbc.Authorizer = sbh.arm.Auth

	// Check if a service bus namespace exists in the resource group
	sbItr, err := sbc.ListByResourceGroupComplete(ctx, sbh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("Failed to list service bus namespaces: %w", err)
	}

	var sbNamespace servicebus.SBNamespace
	if sbItr.NotDone() {
		// A service bus namespace already exists
		sbNamespace = sbItr.Value()
	} else {
		// Generate a random namespace name
		namespaceName := namegenerator.GenerateName("radius-ns")

		// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
		// in something to do with data locality.
		rgc := resources.NewGroupsClient(sbh.arm.SubscriptionID)
		rgc.Authorizer = sbh.arm.Auth

		g, err := rgc.Get(ctx, sbh.arm.ResourceGroup)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		sbNamespaceFuture, err := sbc.CreateOrUpdate(ctx, sbh.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
			Sku: &servicebus.SBSku{
				Name:     servicebus.Basic,
				Tier:     servicebus.SkuTierBasic,
				Capacity: to.Int32Ptr(1),
			},
			Location: g.Location,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		sbNamespace, err = sbNamespaceFuture.Result(sbc)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}
	}

	// store account so we can delete later
	properties["servicebusnamespace"] = *sbNamespace.Name
	properties["servicebusid"] = *sbNamespace.ID

	queueName, ok := properties["servicebusqueue"]
	if !ok {
		return nil, fmt.Errorf("failed to PUT service bus: %w", err)
	}
	qc := servicebus.NewQueuesClient(sbh.arm.SubscriptionID)
	qc.Authorizer = sbh.arm.Auth

	sbq, err := qc.CreateOrUpdate(ctx, sbh.arm.ResourceGroup, *sbNamespace.Name, queueName, servicebus.SBQueue{
		Name: to.StringPtr(queueName),
		SBQueueProperties: &servicebus.SBQueueProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT servicebus queue: %w", err)
	}

	// store db so we can delete later
	properties["queueName"] = *sbq.Name
	return properties, nil
}

func (sbh *serviceBusQueueHandler) Delete(ctx context.Context, properties map[string]string) error {
	namespaceName := properties["servicebusnamespace"]
	queueName := properties["servicebusqueue"]

	qc := servicebus.NewQueuesClient(sbh.arm.SubscriptionID)
	qc.Authorizer = sbh.arm.Auth

	_, err := qc.Delete(ctx, sbh.arm.ResourceGroup, namespaceName, queueName)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus queue: %w", err)
	}

	qItr, err := qc.ListByNamespaceComplete(ctx, sbh.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus queue: %w", err)
	}

	if qItr.NotDone() {
		// There are other queues in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return nil
	}

	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := servicebus.NewNamespacesClient(sbh.arm.SubscriptionID)
	sbc.Authorizer = sbh.arm.Auth

	sbNamespaceFuture, err := sbc.Delete(ctx, sbh.arm.ResourceGroup, namespaceName)

	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	_, err = sbNamespaceFuture.Result(sbc)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	return nil
}

func (kvh *keyVaultHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	if resource.Type != "azure.keyvault" {
		return nil, errors.New("wrong resource type")
	}

	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	return properties, nil
}

func (kvh *keyVaultHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	properties, err := kvh.GetProperties(resource)
	if err != nil {
		return nil, err
	}

	mergeProperties(properties, existing)

	vaultName := namegenerator.GenerateName("kv")

	rgc := resources.NewGroupsClient(kvh.arm.SubscriptionID)
	rgc.Authorizer = kvh.arm.Auth

	g, err := rgc.Get(ctx, kvh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	kvc := keyvault.NewVaultsClient(kvh.arm.SubscriptionID)
	kvc.Authorizer = kvh.arm.Auth
	if err != nil {
		return nil, err
	}

	sc := subscriptions.NewClient()
	sc.Authorizer = kvh.arm.Auth
	s, err := sc.Get(ctx, kvh.arm.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.FromString(*s.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}

	vaultsFuture, err := kvc.CreateOrUpdate(
		ctx,
		kvh.arm.ResourceGroup,
		vaultName,
		keyvault.VaultCreateOrUpdateParameters{
			Location: g.Location,
			Properties: &keyvault.VaultProperties{
				TenantID: &tenantID,
				Sku: &keyvault.Sku{
					Family: to.StringPtr("A"),
					Name:   keyvault.Standard,
				},
				EnableRbacAuthorization: to.BoolPtr(true),
			},
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	err = vaultsFuture.WaitForCompletionRef(ctx, kvc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	kv, err := vaultsFuture.Result(kvc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	// store vault so we can use later
	properties[keyvaultv1alpha1.KeyVaultName] = *kv.Name

	return properties, nil
}

func (kvh *keyVaultHandler) Delete(ctx context.Context, properties map[string]string) error {
	// Delete key vault
	vaultName := properties[keyvaultv1alpha1.KeyVaultName]

	kvClient := keyvault.NewVaultsClient(kvh.arm.SubscriptionID)
	kvClient.Authorizer = kvh.arm.Auth

	_, err := kvClient.Delete(ctx, kvh.arm.ResourceGroup, vaultName)
	if err != nil {
		return fmt.Errorf("failed to DELETE keyvault: %w", err)
	}

	return nil
}

func (pih *podIdentityHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	p := map[string]string{
		"kind":       item.GetKind(),
		"apiVersion": item.GetAPIVersion(),
		"namespace":  item.GetNamespace(),
		"name":       item.GetName(),
	}
	return p, nil
}

func (pih *podIdentityHandler) Put(ctx context.Context, resource workloads.WorkloadResource, existing *db.DeploymentResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return nil, errors.New("inner type was not a map[string]string")
	}

	mergeProperties(properties, existing)

	return properties, nil
}

func (pih *podIdentityHandler) Delete(ctx context.Context, properties map[string]string) error {
	// Delete AAD Pod Identity created
	podIdentityName := properties[containerv1alpha1.PodIdentityName]
	podidentityCluster := properties[containerv1alpha1.PodIdentityCluster]

	mcc := containerservice.NewManagedClustersClient(pih.arm.SubscriptionID)
	mcc.Authorizer = pih.arm.Auth

	// Get the cluster and modify it to remove pod identity
	managedCluster, err := mcc.Get(ctx, pih.arm.ResourceGroup, podidentityCluster)
	if err != nil {
		return fmt.Errorf("failed to get managed cluster: %w", err)
	}

	var identities []containerservice.ManagedClusterPodIdentity
	if managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities == nil {
		// Pod identity does not exist
		return nil
	}

	identities = *managedCluster.PodIdentityProfile.UserAssignedIdentities

	var i int
	var identity containerservice.ManagedClusterPodIdentity
	for i, identity = range *managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities {
		if *identity.Name == podIdentityName {
			break
		}
	}

	// Remove the pod identity at the matching index
	identities = append(identities[:i], identities[i+1:]...)

	mcFuture, err := mcc.CreateOrUpdate(ctx, pih.arm.ResourceGroup, podidentityCluster, containerservice.ManagedCluster{
		ManagedClusterProperties: &containerservice.ManagedClusterProperties{
			PodIdentityProfile: &containerservice.ManagedClusterPodIdentityProfile{
				Enabled:                   to.BoolPtr(true),
				AllowNetworkPluginKubenet: to.BoolPtr(false),
				UserAssignedIdentities:    &identities,
			},
		},
		Location: managedCluster.Location,
	})

	if err != nil {
		return fmt.Errorf("failed to delete pod identity on the cluster: %w", err)
	}

	err = mcFuture.WaitForCompletionRef(ctx, mcc.Client)
	if err != nil {
		return fmt.Errorf("failed to delete pod identity on the cluster: %w", err)
	}

	// Delete the managed identity
	err = pih.deleteManagedIdentity(ctx, *identity.Identity.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}

func (pih *podIdentityHandler) deleteManagedIdentity(ctx context.Context, msiResourceID string) error {
	msiClient := msi.NewUserAssignedIdentitiesClient(pih.arm.SubscriptionID)
	msiClient.Authorizer = pih.arm.Auth
	msiResource, err := azure.ParseResourceID(msiResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}
	resp, err := msiClient.Delete(ctx, pih.arm.ResourceGroup, msiResource.ResourceName)
	if err != nil || (resp.StatusCode != 200 && resp.StatusCode != 204) {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
