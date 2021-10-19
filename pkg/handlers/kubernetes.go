// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesHandler(k8s client.Client) ResourceHandler {
	return &kubernetesHandler{k8s: k8s}
}

type kubernetesHandler struct {
	k8s client.Client
}

func (handler *kubernetesHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	item, err := convertToUnstructured(*options.Resource)
	if err != nil {
		return nil, err
	}

	err = handler.PatchNamespace(ctx, item.GetNamespace())
	if err != nil {
		return nil, err
	}

	// For a Kubernetes resource we only need to store the ObjectMeta and TypeMeta data
	properties := map[string]string{
		KubernetesKindKey:       item.GetKind(),
		KubernetesAPIVersionKey: item.GetAPIVersion(),
		KubernetesNamespaceKey:  item.GetNamespace(),
		ResourceName:            item.GetName(),
	}

	if options.Resource.Deployed {
		// This resource is deployed in the Render process
		// TODO: This will eventually change
		// For now, no need to process any further
		return properties, nil
	}
	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.ResourceIdentity{
		Kind: resourcemodel.IdentityKindKubernetes,
		Data: resourcemodel.KubernetesIdentity{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Kind:       item.GetKind(),
			APIVersion: item.GetAPIVersion(),
		},
	}

	return properties, err
}

func (handler *kubernetesHandler) PatchNamespace(ctx context.Context, namespace string) error {
	// Ensure that the namespace exists that we're able to operate upon.
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespace,
				"labels": map[string]interface{}{
					kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				},
			},
		},
	}

	err := handler.k8s.Patch(ctx, ns, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		// we consider this fatal - without a namespace we won't be able to apply anything else
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}

func (handler *kubernetesHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ResourceName],
			},
		},
	}

	// TODO make sure deletion of a patch works correctly here, can't just delete
	//
	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}

func convertToUnstructured(resource outputresource.OutputResource) (unstructured.Unstructured, error) {
	if resource.ResourceKind != resourcekinds.Kubernetes {
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

func NewKubernetesHealthHandler(k8s client.Client) HealthHandler {
	return &kubernetesHealthHandler{k8s: k8s}
}

type kubernetesHealthHandler struct {
	k8s client.Client
}

func (handler *kubernetesHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
