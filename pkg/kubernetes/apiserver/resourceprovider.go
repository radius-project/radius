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
	"github.com/Azure/radius/pkg/model"
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

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider(appmodel model.ApplicationModel, client controller_runtime.Client) resourceprovider.ResourceProvider {
	return &rp{AppModel: appmodel, client: client, namespace: "default"}
}

type rp struct {
	client    controller_runtime.Client
	namespace string
	AppModel  model.ApplicationModel
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
		id := id
		id.Types[len(id.Types)-1].Name = item.Name
		converted, err := NewRestApplicationResource(id, item)
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

	converted, err := NewKubernetesApplicationResource(id, application)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	converted.Namespace = r.namespace
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
			Name:      id.Types[len(id.Types)-1].Name,
		},
	}
	err = r.client.Delete(ctx, &item)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	return rest.NewNoContentResponse(), nil
}

func (r *rp) ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	application := radiusv1alpha3.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: id.Types[len(id.Types)-2].Name}, &application)
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
			Group:   "radius.dev",
			Version: "v1alpha3",
			Kind:    kubernetesType + "List",
		})
		err = r.client.List(ctx, &items, controller_runtime.InNamespace(r.namespace), controller_runtime.MatchingLabels{
			kubernetes.LabelRadiusApplication: id.Types[len(id.Types)-2].Name,
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

			// The last type is '{ Type: RadiusResource Name: '' }
			//
			// Chop that off and add the *real* type/name
			id := id.Truncate().Append(azresources.ResourceType{Type: armType, Name: resource.Spec.Resource})
			converted, err := NewRestRadiusResource(id, resource)
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
	err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: id.Types[len(id.Types)-2].Name}, &application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	output := resourceprovider.RadiusResourceList{}

	items := unstructured.UnstructuredList{}
	items.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(id.Types[len(id.Types)-1].Type) + "List",
	})
	err = r.client.List(ctx, &items, controller_runtime.InNamespace(r.namespace), controller_runtime.MatchingLabels{
		kubernetes.LabelRadiusApplication: id.Name(),
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

		id := id.Append(azresources.ResourceType{Type: id.Types[len(id.Types)-1].Type, Name: resource.Spec.Resource})
		converted, err := NewRestRadiusResource(id, resource)
		if err != nil {
			return nil, err
		}

		output.Value = append(output.Value, converted)
	}

	return rest.NewOKResponse(output), nil
}

func (r *rp) GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	application := radiusv1alpha3.Application{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: id.Types[len(id.Types)-2].Name}, &application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		return rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	fmt.Println()

	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(id.Types[len(id.Types)-1].Type),
	})
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(id.Types[len(id.Types)-2].Name, id.Types[len(id.Types)-1].Name)}, &item)
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

	output, err := NewRestRadiusResource(id, resource)
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

	item, err := NewKubernetesRadiusResource(id, resource)
	if err != nil {
		return nil, err // Unexpected error, the payload has already been validated.
	}

	item.SetNamespace(r.namespace)
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(id.Types[len(id.Types)-1].Type),
	})
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

	output, err := NewRestRadiusResource(id, k8sOutput)
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

	item := unstructured.Unstructured{}
	item.SetNamespace(r.namespace)
	item.SetName(kubernetes.MakeResourceName(id.Types[len(id.Types)-2].Name, id.Types[len(id.Types)-1].Name))
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(id.Types[len(id.Types)-1].Type),
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

	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(id.Types[len(id.Types)-1].Type),
	})
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(id.Types[len(id.Types)-2].Name, id.Types[len(id.Types)-1].Name)}, &item)
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

	output, err := NewRestRadiusResource(id, resource)
	if err != nil {
		return nil, err
	}

	if !rest.IsTeminalStatus(rest.OperationStatus(output.Properties["provisioningState"].(string))) {
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Internal,
				Message: "resource is not ready yet",
				Target:  id.ID,
			},
		}), nil
	}

	cv, err := converters.GetComputedValues(resource.Status)
	computedValues := map[string]interface{}{}
	for k, v := range cv {
		computedValues[k] = v.Value
	}

	// The 'SecretValues' we store as part of the resource status (from render output) are references
	// to secrets, we need to fetch the values and pass them to the renderer.
	secretValues, err := converters.GetSecretValues(resource.Status)
	if err != nil {
		return nil, err
	}

	values := map[string]interface{}{}
	for k, v := range secretValues {
		// cloud, ok := resource.Status.CloudResources[v.LocalID]
		// if ok {
		// 	// This is an Azure resource
		// 	arm, err := armauth.GetArmAuthorizer()
		// 	if err != nil {
		// 		return nil, fmt.Errorf("failed to authenticate with Azure: %w", err)
		// 	}

		// 	identity := resourcemodel.NewARMIdentity(strings.Split(cloud.Identity, "@")[0], strings.Split(cloud.Identity, "@")[1])

		// 	azureSecretClient := renderers.NewSecretValueClient(arm)
		// 	value, err := azureSecretClient.FetchSecret(ctx, identity, v.Action, v.ValueSelector)
		// 	if err != nil {
		// 		return nil, err
		// 	}

		// 	if v.Transformer != "" {
		// 		outputResourceType, err := r.AppModel.LookupOutputResource(v.Transformer)
		// 		if err != nil {
		// 			return nil, err
		// 		}

		// 		transformer := outputResourceType.SecretValueTransformer
		// 		if transformer == nil {
		// 			return nil, fmt.Errorf("output resource %q has no secret value transformer", v.Transformer)
		// 		}

		// 		dependency := renderers.RendererDependency{
		// 			ComputedValues: computedValues,
		// 			ResourceID:     id,
		// 			Definition:     output.Properties,
		// 		}

		// 		value, err = transformer.Transform(ctx, dependency, value)
		// 		if err != nil {
		// 			return nil, err
		// 		}
		// 	}

		// 	values[k] = value
		// 	continue
		// }

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
	item := unstructured.Unstructured{}
	item.SetGroupVersionKind(k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(targetID.Types[len(targetID.Types)-1].Type),
	})
	err = r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: kubernetes.MakeResourceName(targetID.Types[len(targetID.Types)-2].Name, targetID.Types[len(targetID.Types)-1].Name)}, &item)
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

	output, err := NewRestRadiusResource(targetID, resource)
	if err != nil {
		return nil, err
	}

	if rest.IsTeminalStatus(rest.OperationStatus(output.Properties["provisioningState"].(string))) {
		return rest.NewOKResponse(output), nil
	}

	// Operation is still processing.
	// The ARM-RPC spec wants us to keep returning 202 from here until the operation is complete.
	return rest.NewAcceptedAsyncResponse(output, id.ID), nil
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
