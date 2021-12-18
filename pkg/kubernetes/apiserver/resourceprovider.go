// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrUnsupportedResourceType = errors.New("unsupported resource type")

const (
	RadiusGroup   = "radius.dev"
	RadiusVersion = "v1alpha3"
)

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider(client controller_runtime.Client) resourceprovider.ResourceProvider {
	return &rp{client: client, namespace: "default"}
}

type rp struct {
	client    controller_runtime.Client
	namespace string
}

// As a general design principle, returning an error from the RP signals an internal error (500).
// Code paths that validate input should return a rest.Response.

func (r *rp) ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	items := radiusv1alpha3.ApplicationList{}
	err = r.client.List(ctx, &items, controller_runtime.InNamespace(r.namespace))
	if err != nil {
		return nil, err
	}

	output := resourceprovider.ApplicationResourceList{}
	for _, item := range items.Items {
		typeName := r.getApplicationTypeFromApplicationResourceId(id) // Should always be Application
		// Add name to resource ID, by removing the last type/name and appending
		// the actual part.
		newId := id.Truncate().Append(azresources.ResourceType{Type: typeName, Name: item.Name})
		converted, err := NewRestApplicationResource(newId, item)
		if err != nil {
			return nil, err
		}
		output.Value = append(output.Value, converted)
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	item := radiusv1alpha3.Application{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: id.Name()}, &item)
	if err != nil && controller_runtime.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output, err := NewRestApplicationResource(id, item)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	application := resourceprovider.ApplicationResource{}
	err = json.Unmarshal(body, &application)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	converted, err := NewKubernetesApplicationResource(id, application, r.namespace)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	err = r.client.Patch(ctx, &converted, controller_runtime.Apply, controller_runtime.FieldOwner("rad-api-server"))
	if err != nil {
		return nil, err
	}

	output, err := NewRestApplicationResource(id, converted)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateApplicationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	item := radiusv1alpha3.Application{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.namespace,
			Name:      r.getApplicationNameFromApplicationResourceId(id),
		},
	}
	err = r.client.Delete(ctx, &item)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Resource name is RadiusResource
	err := r.validateResourceType(id)

	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	application := radiusv1alpha3.Application{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: r.getApplicationNameFromResourceId(id)}, &application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output := resourceprovider.RadiusResourceList{}

	for armType, kubernetesType := range armtemplate.GetSupportedTypes() {
		if armType == "Application" {
			continue
		}

		items := unstructured.UnstructuredList{}
		items.SetGroupVersionKind(k8sschema.GroupVersionKind{
			Group:   RadiusGroup,
			Version: RadiusVersion,
			Kind:    kubernetesType + "List",
		})
		err = r.client.List(ctx, &items, controller_runtime.InNamespace(r.namespace), controller_runtime.MatchingLabels{
			kubernetes.LabelRadiusApplication: r.getApplicationNameFromResourceId(id),
		})
		if err != nil {
			return nil, err
		}

		for _, item := range items.Items {
			resource := radiusv1alpha3.Resource{}
			b, err := item.MarshalJSON()
			if err != nil {
				return nil, err
			}

			err = json.Unmarshal(b, &resource)
			if err != nil {
				return nil, err
			}

			converted, err := NewRestRadiusResource(resource)
			if err != nil {
				return nil, err
			}

			output.Value = append(output.Value, converted)
		}
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	application := radiusv1alpha3.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: r.getApplicationNameFromResourceId(id)}, &application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output := resourceprovider.RadiusResourceList{}

	kind, ok := armtemplate.GetKindFromArmType(r.getResourceTypeFromResourceId(id))
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", r.getResourceTypeFromResourceId(id))
	}
	kindlist := kind + "List"
	items := unstructured.UnstructuredList{}
	items.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kindlist,
	})
	err = r.client.List(ctx, &items, controller_runtime.InNamespace(r.namespace), controller_runtime.MatchingLabels{
		kubernetes.LabelRadiusApplication: r.getApplicationNameFromResourceId(id),
	})
	if err != nil {
		return nil, err
	}

	for _, item := range items.Items {
		resource := radiusv1alpha3.Resource{}
		b, err := item.MarshalJSON()
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(b, &resource)
		if err != nil {
			return nil, err
		}

		converted, err := NewRestRadiusResource(resource)
		if err != nil {
			return nil, err
		}

		output.Value = append(output.Value, converted)
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	application := radiusv1alpha3.Application{}

	err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: r.getApplicationNameFromResourceId(id)}, &application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	kind, ok := armtemplate.GetKindFromArmType(r.getResourceTypeFromResourceId(id))
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", r.getResourceTypeFromResourceId(id))
	}

	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kind,
	})

	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(r.getApplicationNameFromResourceId(id), r.getResourceNameFromResourceId(id))}, &item)
	if err != nil {
		return nil, err
	}

	resource := radiusv1alpha3.Resource{}
	b, err := item.MarshalJSON()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	output, err := NewRestRadiusResource(resource)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	resource := resourceprovider.RadiusResource{}
	err = json.Unmarshal(body, &resource)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	kind, ok := armtemplate.GetKindFromArmType(r.getResourceTypeFromResourceId(id))
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", r.getResourceTypeFromResourceId(id))
	}
	item, err := NewKubernetesRadiusResource(id, resource, r.namespace, k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kind,
	})
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	err = r.client.Patch(ctx, &item, controller_runtime.Apply, controller_runtime.FieldOwner("rad-api-server"))
	if err != nil {
		return nil, err
	}

	generation := item.GetGeneration()
	oid := id.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: fmt.Sprintf("%d", generation)})

	k8sOutput := radiusv1alpha3.Resource{}
	b, err := item.MarshalJSON()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	output, err := NewRestRadiusResource(k8sOutput)
	if err != nil {
		return nil, err
	}

	return rest.NewAcceptedAsyncResponse(output, oid.ID), nil
}

func (r *rp) DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	kind, ok := armtemplate.GetKindFromArmType(r.getResourceTypeFromResourceId(id))
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", r.getResourceTypeFromResourceId(id))
	}

	item := unstructured.Unstructured{}
	item.SetNamespace(r.namespace)
	item.SetName(kubernetes.MakeResourceName(r.getApplicationNameFromResourceId(id), r.getResourceNameFromResourceId(id)))
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kind,
	})
	err = r.client.Delete(ctx, &item)
	if err != nil {
		return nil, err
	}

	// For now we treat deletion as synchronous.
	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListSecrets(ctx context.Context, input resourceprovider.ListSecretsInput) (rest.Response, error) {
	id, err := azresources.Parse(input.TargetID)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	err = r.validateResourceType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	kind, ok := armtemplate.GetKindFromArmType(r.getResourceTypeFromResourceId(id))
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", r.getResourceTypeFromResourceId(id))
	}

	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kind,
	})
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(r.getApplicationNameFromResourceId(id), r.getResourceNameFromResourceId(id))}, &item)
	if err != nil {
		return nil, err
	}

	resource := radiusv1alpha3.Resource{}
	b, err := item.MarshalJSON()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	output, err := NewRestRadiusResource(resource)
	if err != nil {
		return nil, err
	}

	// Check if the resource is provisioned and ready
	if state, ok := output.Properties["state"]; ok && !rest.IsTeminalStatus(rest.OperationStatus(state.(rest.ResourceStatus).ProvisioningState)) {
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: "resource is not ready yet",
				Target:  id.ID,
			},
		}), nil
	}

	// The 'SecretValues' we store as part of the resource status (from render output) are references
	// to secrets, we need to fetch the values and pass them to the renderer.
	secretValues, err := converters.GetSecretValues(resource.Status)
	if err != nil {
		return nil, err
	}

	values := map[string]interface{}{}
	for k, v := range secretValues {

		_, ok := resource.Status.Resources[v.LocalID]
		if ok {
			// This is an Kubernetes secret
			kubernetesSecretClient := converters.SecretClient{Client: r.client}
			value, err := kubernetesSecretClient.LookupSecretValue(ctx, resource.Status, v)
			if err != nil {
				return nil, err
			}

			values[k] = value
			continue
		}
	}

	return rest.NewOKResponse(values), nil
}

func (r *rp) GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	err := r.validateOperationType(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	targetID := id.Truncate()

	kind, ok := armtemplate.GetKindFromArmType(targetID.Types[len(targetID.Types)-1].Type)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", targetID.Types[len(targetID.Types)-1].Type)
	}

	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   RadiusGroup,
		Version: RadiusVersion,
		Kind:    kind,
	})
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(r.getApplicationNameFromResourceId(targetID), r.getResourceNameFromResourceId(targetID))}, &item)
	if err != nil {
		return nil, err
	}

	resource := radiusv1alpha3.Resource{}
	b, err := item.MarshalJSON()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	output, err := NewRestRadiusResource(resource)
	if err != nil {
		return nil, err
	}

	if state, ok := output.Properties["state"]; ok && !rest.IsTeminalStatus(rest.OperationStatus(state.(rest.ResourceStatus).ProvisioningState)) {
		// Operation is still processing.
		// The ARM-RPC spec wants us to keep returning 202 from here until the operation is complete.
		return rest.NewAcceptedAsyncResponse(output, id.ID), nil
	}

	return rest.NewOKResponse(output), nil

}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateApplicationType(id azresources.ResourceID) error {
	if len(id.Types) != 2 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

func (r *rp) GetSwaggerDoc(ctx context.Context) (rest.Response, error) {

	// We must return at least one resource, otherwise
	// there will be errors on the client saying: memcache.go:196] couldn't get resource list for api.radius.dev/v1alpha3:
	// 0-length response with status code: 200 and content type: application/json
	// So return a dummy resource with no ability to call get, list, etc.
	resp := metav1.APIResourceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: "v1",
		},
		GroupVersion: k8sschema.GroupVersion{Group: "api.radius.dev", Version: RadiusVersion}.String(),
		APIResources: []metav1.APIResource{
			{
				Name:         "apiradiuss",
				Kind:         "APIRadius",
				SingularName: "apiradius",
				Namespaced:   true,
			},
		},
	}

	return rest.NewOKResponse(resp), nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateResourceType(id azresources.ResourceID) error {
	if len(id.Types) != 3 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) ||
		!schema.HasType(id.Types[2].Type) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

// We don't really expect an invalid type to get through ARM's routing
// but we're testing it anyway to catch bugs.
func (r *rp) validateOperationType(id azresources.ResourceID) error {
	if len(id.Types) != 4 ||
		!strings.EqualFold(id.Types[0].Type, azresources.CustomProvidersResourceProviders) ||
		!strings.EqualFold(id.Types[1].Type, azresources.ApplicationResourceType) ||
		!schema.HasType(id.Types[2].Type) ||
		!strings.EqualFold(id.Types[3].Type, azresources.OperationResourceType) {
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

func (r *rp) getApplicationNameFromApplicationResourceId(id azresources.ResourceID) string {
	return id.Types[len(id.Types)-1].Name
}

func (r *rp) getApplicationTypeFromApplicationResourceId(id azresources.ResourceID) string {
	return id.Types[len(id.Types)-1].Type
}

func (r *rp) getResourceNameFromResourceId(id azresources.ResourceID) string {
	return id.Types[len(id.Types)-1].Name
}

func (r *rp) getResourceTypeFromResourceId(id azresources.ResourceID) string {
	return id.Types[len(id.Types)-1].Type
}

func (r *rp) getApplicationNameFromResourceId(id azresources.ResourceID) string {
	return id.Types[len(id.Types)-2].Name
}
