// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func GetDaprStateStoreGeneric(resource datamodel.DaprStateStore, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	properties := resource.Properties.DaprStateStoreGeneric

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
	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resource.Name, namespace)
	if err != nil {
		return nil, err
	}

	output := outputresource.OutputResource{
		LocalID: outputresource.LocalIDDaprComponent,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprComponent,
			Provider: providers.ProviderKubernetes,
		},
		Resource: &daprGenericResource,
	}

	return []outputresource.OutputResource{output}, nil
}
