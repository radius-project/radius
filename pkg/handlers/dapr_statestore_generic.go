// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	GenericDaprStateStoreTypeKey     = "type"
	GenericDaprStateStoreVersionKey  = "version"
	GenericDaprStateStoreMetadataKey = "metadata"
)

func NewDaprStateStoreGenericHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreGenericHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreGenericHandler struct {
	kubernetesHandler
	arm armauth.ArmConfig
	k8s client.Client
}

func constructDaprStateStore(properties map[string]string, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata to a map for easier access
	metadata := map[string]interface{}{}
	err := json.Unmarshal([]byte(properties[GenericDaprStateStoreMetadataKey]), &metadata)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/supported-state-stores/
	yamlListItems := []map[string]interface{}{}
	for k, v := range metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	// Translate into Dapr State Store schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      resourceName,
				"namespace": appName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     properties[GenericDaprStateStoreTypeKey],
				"version":  properties[GenericDaprStateStoreVersionKey],
				"metadata": yamlListItems,
			},
		},
	}
	return item, nil
}

func (handler *daprStateStoreGenericHandler) patchDaprStateStore(ctx context.Context, options *PutOptions, properties map[string]string) (unstructured.Unstructured, error) {
	err := handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	item, err := constructDaprStateStore(properties, options.ApplicationName, options.ResourceName)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	return item, nil
}

func (handler *daprStateStoreGenericHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	item, err := handler.patchDaprStateStore(ctx, options, properties)
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

	return properties, nil
}

func (handler *daprStateStoreGenericHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      properties[KubernetesNameKey],
				"namespace": properties[KubernetesNamespaceKey],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	return nil
}

func NewDaprStateStoreGenericHealthHandler(arm armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprStateStoreGenericHealthHandler{
		arm: arm,
		k8s: k8s,
	}
}

type daprStateStoreGenericHealthHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreGenericHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
