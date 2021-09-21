// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewVolumeHandler(k8s client.Client) ResourceHandler {
	return &volumeHandler{k8s: k8s}
}

type volumeHandler struct {
	k8s client.Client
}

func (handler *volumeHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
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
		ComponentNameKey:        item.GetName(),
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

	options.Resource.Info = outputresource.K8sInfo{
		Name:       item.GetName(),
		Namespace:  item.GetNamespace(),
		Kind:       item.GetKind(),
		APIVersion: item.GetAPIVersion(),
	}

	return properties, err
}

func (handler *volumeHandler) PatchNamespace(ctx context.Context, namespace string) error {
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

func (handler *volumeHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ComponentNameKey],
			},
		},
	}

	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}

func NewVolumeHealthHandler(k8s client.Client) HealthHandler {
	return &volumeHealthHandler{k8s: k8s}
}

type volumeHealthHandler struct {
	k8s client.Client
}

func (handler *volumeHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
