// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

func GetDaprSecretStoreAzureGeneric(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	properties := radclient.DaprSecretStoreProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	daprGeneric := dapr.DaprGeneric{
		Type:     properties.Type,
		Version:  properties.Version,
		Metadata: properties.Metadata,
	}

	err = dapr.ValidateDaprGenericObject(daprGeneric)
	if err != nil {
		return nil, err
	}

	// Convert metadata to string
	metadataSerialized, err := json.Marshal(properties.Metadata)
	if err != nil {
		return nil, err
	}

	// generate data we can use to connect to a Storage Account
	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprSecretStoreGeneric,
		ResourceKind: resourcekinds.DaprSecretStoreGeneric,
		Resource: map[string]string{
			handlers.KubernetesNameKey:       resource.ResourceName,
			handlers.KubernetesNamespaceKey:  resource.ApplicationName,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",
			handlers.ResourceName:            resource.ResourceName,

			handlers.GenericDaprTypeKey:     *properties.Type,
			handlers.GenericDaprVersionKey:  *properties.Version,
			handlers.GenericDaprMetadataKey: string(metadataSerialized),
		},
	}

	return []outputresource.OutputResource{output}, nil
}

func GetDaprSecretStoreKubernetesGeneric(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	properties := radclient.DaprSecretStoreProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	daprGeneric := dapr.DaprGeneric{
		Type:     properties.Type,
		Version:  properties.Version,
		Metadata: properties.Metadata,
	}

	err = dapr.ValidateDaprGenericObject(daprGeneric)
	if err != nil {
		return nil, err
	}

	secretstoreResource, err := dapr.ConstructDaprGeneric(daprGeneric, resource.ApplicationName, resource.ResourceName)
	if err != nil {
		return nil, err
	}

	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprSecretStoreGeneric,
		ResourceKind: resourcekinds.Kubernetes,
		Resource:     &secretstoreResource,
	}

	return []outputresource.OutputResource{output}, nil
}
