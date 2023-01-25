// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"fmt"
	"sort"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

type SecretStoreFunc = func(resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]rpv1.OutputResource, error)

var SupportedSecretStoreModes = map[string]SecretStoreFunc{
	string(datamodel.LinkModeValues): GetDaprSecretStoreGeneric,
}
var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	SecretStores map[string]SecretStoreFunc
}

func (r Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprSecretStore)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretStoreFunc := r.SecretStores[string(properties.Mode)]
	if secretStoreFunc == nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid secret store mode, Supported mode values: %s", getAlphabeticallySortedKeys(r.SecretStores)))
	}
	var applicationName string
	if properties.Application != "" {
		applicationID, err := renderers.ValidateApplicationID(properties.Application)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		applicationName = applicationID.Name()
	}

	resources, err := secretStoreFunc(*resource, applicationName, options.Namespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources: resources,
		ComputedValues: map[string]renderers.ComputedValueReference{
			renderers.ComponentNameKey: {
				Value: kubernetes.NormalizeResourceName(resource.Name),
			},
		},
		SecretValues: map[string]rpv1.SecretValueReference{},
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

func GetDaprSecretStoreGeneric(resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]rpv1.OutputResource, error) {
	properties := resource.Properties
	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	return GetDaprGeneric(daprGeneric, resource, applicationName, namespace)
}

func GetDaprGeneric(daprGeneric dapr.DaprGeneric, resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]rpv1.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
	}

	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resource.Name, namespace, resource.ResourceTypeName())
	if err != nil {
		return nil, err
	}

	output := rpv1.OutputResource{
		LocalID: rpv1.LocalIDDaprComponent,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprComponent,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Resource: &daprGenericResource,
	}

	return []rpv1.OutputResource{output}, nil
}
