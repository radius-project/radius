// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type SecretStoreFunc = func(resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]outputresource.OutputResource, error)

var SupportedSecretStoreKindValues = map[string]SecretStoreFunc{
	resourcekinds.DaprGeneric: GetDaprSecretStoreGeneric,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	SecretStores map[string]SecretStoreFunc
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprSecretStore)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretStoreFunc := r.SecretStores[string(properties.Kind)]
	if secretStoreFunc == nil {
		return renderers.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %s", properties.Kind, getAlphabeticallySortedKeys(r.SecretStores))
	}
	var applicationName string
	if resource.Properties.Application != "" {
		applicationID, err := resources.Parse(resource.Properties.Application)
		if err != nil {
			return renderers.RendererOutput{}, errors.New("the 'application' field must be a valid resource id")
		}
		applicationName = applicationID.Name()
	}

	resoures, err := secretStoreFunc(*resource, applicationName, options.Namespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources: resoures,
		ComputedValues: map[string]renderers.ComputedValueReference{
			"secretStoreName": {
				Value: kubernetes.MakeResourceName(applicationName, resource.Name),
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

func GetDaprSecretStoreGeneric(resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	properties := resource.Properties
	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	return GetDaprGeneric(daprGeneric, resource, applicationName, namespace)
}

func GetDaprGeneric(daprGeneric dapr.DaprGeneric, resource datamodel.DaprSecretStore, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
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
