// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"fmt"
	"sort"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
)

type SecretStoreFunc = func(conv.DataModelInterface) ([]outputresource.OutputResource, error)

var SupportedSecretStoreKindValues = map[string]SecretStoreFunc{
	resourcekinds.DaprGeneric: GetDaprSecretStoreGeneric,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	SecretStores map[string]SecretStoreFunc
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface) (rp.RendererOutput, error) {
	resource, ok := dm.(datamodel.DaprSecretStore)
	if !ok {
		return rp.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretStoreFunc := r.SecretStores[string(properties.Kind)]
	if secretStoreFunc == nil {
		return rp.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %s", properties.Kind, getAlphabeticallySortedKeys(r.SecretStores))
	}
	resoures, err := secretStoreFunc(resource)
	if err != nil {
		return rp.RendererOutput{}, err
	}
	return rp.RendererOutput{
		Resources: resoures,
		ComputedValues: map[string]rp.ComputedValueReference{
			"secretStoreName": {
				Value: resource.Name,
			},
		},
		SecretValues: map[string]rp.SecretValueReference{},
	}, nil

}

func getAlphabeticallySortedKeys(store map[string]SecretStoreFunc) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}

func GetDaprSecretStoreGeneric(dm conv.DataModelInterface) ([]outputresource.OutputResource, error) {
	resource, ok := dm.(datamodel.DaprSecretStore)
	if !ok {
		return nil, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties
	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	return GetDaprGeneric(daprGeneric, dm)
}

func GetDaprGeneric(daprGeneric dapr.DaprGeneric, dm conv.DataModelInterface) ([]outputresource.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
	}
	resource, ok := dm.(datamodel.DaprSecretStore)
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
