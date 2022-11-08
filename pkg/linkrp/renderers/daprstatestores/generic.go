// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

func GetDaprStateStoreGeneric(resource datamodel.DaprStateStore, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	properties := resource.Properties

	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	return getDaprGeneric(daprGeneric, resource, applicationName, namespace)
}

func getDaprGeneric(daprGeneric dapr.DaprGeneric, dm conv.DataModelInterface, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
	}
	resource, ok := dm.(datamodel.DaprStateStore)
	if !ok {
		return nil, conv.ErrInvalidModelConversion
	}
	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resource.Name, namespace, ResourceType)
	if err != nil {
		return nil, err
	}

	output := outputresource.OutputResource{
		LocalID: outputresource.LocalIDDaprComponent,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprComponent,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Resource: &daprGenericResource,
	}

	return []outputresource.OutputResource{output}, nil
}
