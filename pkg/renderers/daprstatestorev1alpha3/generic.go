// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetDaprStateStoreAzureGeneric(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	properties := radclient.DaprStateStoreGenericResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	if properties.Type == nil || *properties.Type == "" {
		return nil, errors.New("No type specified for generic Dapr State Store component")
	}

	if properties.Version == nil || *properties.Version == "" {
		return nil, errors.New("No Dapr component version specified for generic State Store component")
	}

	if properties.Metadata == nil || len(properties.Metadata) == 0 {
		return nil, fmt.Errorf("No metadata specified for Dapr State Store component of type %s", *properties.Type)
	}

	// Convert metadata to string
	metadataSerialized, err := json.Marshal(properties.Metadata)
	if err != nil {
		return nil, err
	}

	// generate data we can use to connect to a Storage Account
	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprStateStoreGeneric,
		ResourceKind: resourcekinds.DaprStateStoreGeneric,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.KubernetesNameKey:       resource.ResourceName,
			handlers.KubernetesNamespaceKey:  resource.ApplicationName,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",
			handlers.ResourceName:            resource.ResourceName,

			handlers.GenericDaprStateStoreTypeKey:     *properties.Type,
			handlers.GenericDaprStateStoreVersionKey:  *properties.Version,
			handlers.GenericDaprStateStoreMetadataKey: string(metadataSerialized),
		},
	}

	return []outputresource.OutputResource{output}, nil
}

func GetDaprStateStoreKubernetesGeneric(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	properties := radclient.DaprStateStoreGenericResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	if properties.Type == nil || *properties.Type == "" {
		return nil, errors.New("No type specified for generic Dapr State Store component")
	}

	if properties.Version == nil || *properties.Version == "" {
		return nil, errors.New("No Dapr component version specified for generic State Store component")
	}

	if properties.Metadata == nil || len(properties.Metadata) == 0 {
		return nil, fmt.Errorf("No metadata specified for Dapr State Store component of type %s", *properties.Type)
	}

	pubsubResource, err := constructDaprStateStore(properties, resource.ApplicationName, resource.ResourceName)
	if err != nil {
		return nil, err
	}

	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprStateStoreGeneric,
		ResourceKind: resourcekinds.Kubernetes,
		Managed:      false,
		Resource:     &pubsubResource,
	}

	return []outputresource.OutputResource{output}, nil
}

func constructDaprStateStore(properties radclient.DaprStateStoreGenericResourceProperties, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/supported-pubsub/
	yamlListItems := []map[string]interface{}{}
	for k, v := range properties.Metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	// Translate into Dapr State Store schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     *properties.Type,
				"version":  *properties.Version,
				"metadata": yamlListItems,
			},
		},
	}
	return item, nil
}
