// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	GenericPubSubTypeKey     = "pubsubtype"
	GenericPubSubVersionKey  = "version"
	GenericPubSubMetadataKey = "metadata"
)

func NewDaprPubSubGenericHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprPubSubGenericHandler{
		armHandler:        armHandler{arm: arm},
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		k8s:               k8s,
	}
}

type daprPubSubGenericHandler struct {
	armHandler
	kubernetesHandler
	k8s client.Client
}

func (handler *daprPubSubGenericHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)
	item, err := handler.PatchDaprPubSub(ctx, properties, *options)
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

func constructPubSubResource(properties map[string]string, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata to a map for easier access
	metadata := map[string]interface{}{}
	err := json.Unmarshal([]byte(properties[GenericPubSubMetadataKey]), &metadata)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/supported-pubsub/
	yamlListItems := []map[string]interface{}{}
	for k, v := range metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ResourceName],
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     properties[GenericPubSubTypeKey],
				"version":  properties[GenericPubSubVersionKey],
				"metadata": yamlListItems,
			},
		},
	}

	return item, nil
}

func (handler *daprPubSubGenericHandler) PatchDaprPubSub(ctx context.Context, properties map[string]string, options PutOptions) (unstructured.Unstructured, error) {
	err := handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	// Convert the metadata to a map for easier access
	item, err := constructPubSubResource(properties, options.ApplicationName, options.ResourceName)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	return item, nil
}

func (handler *daprPubSubGenericHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

	err := handler.DeleteDaprPubSub(ctx, properties)
	if err != nil {
		return err
	}

	return nil
}

func (handler *daprPubSubGenericHandler) DeleteDaprPubSub(ctx context.Context, properties map[string]string) error {
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

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}

func NewDaprPubSubGenericHealthHandler(arm armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprPubSubGenericHealthHandler{
		armHandler:        armHandler{arm: arm},
		kubernetesHandler: kubernetesHandler{k8s: k8s},
	}
}

type daprPubSubGenericHealthHandler struct {
	armHandler
	kubernetesHandler
}

func (handler *daprPubSubGenericHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
