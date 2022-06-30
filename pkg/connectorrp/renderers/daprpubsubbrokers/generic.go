// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func GetDaprPubSubGeneric(dm conv.DataModelInterface) (renderers.RendererOutput, error) {
	resource, ok := dm.(datamodel.DaprPubSubBroker)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties.DaprPubSubGeneric

	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	outputResources, err := getDaprGeneric(daprGeneric, dm)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: nil,
		SecretValues:   nil,
	}, nil

}

func getDaprGeneric(daprGeneric dapr.DaprGeneric, dm conv.DataModelInterface) ([]outputresource.OutputResource, error) {
	err := dapr.ValidateDaprGenericObject(daprGeneric)
	if err != nil {
		return nil, err
	}
	resource, ok := dm.(datamodel.DaprPubSubBroker)
	if !ok {
		return nil, conv.ErrInvalidModelConversion
	}
	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, resource.Properties.Application, resource.Name)
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
