// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
)

type StateStoreFunc = func(renderers.RendererResource, Properties) ([]outputresource.OutputResource, error)

var SupportedAzureStateStoreKindValues = map[string]StateStoreFunc{
	"any":                      GetDaprStateStoreAzureStorage,
	"state.azure.tablestorage": GetDaprStateStoreAzureStorage,
	"state.sqlserver":          GetDaprStateStoreSQLServer,
}

var SupportedKubernetesStateStoreKindValues = map[string]StateStoreFunc{
	"any":         GetDaprStateStoreKubernetesRedis,
	"state.redis": GetDaprStateStoreKubernetesRedis,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	StateStores map[string]StateStoreFunc
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := Properties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if r.StateStores == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}

	stateStoreFunc := r.StateStores[properties.Kind]
	if stateStoreFunc == nil {
		return renderers.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %s", properties.Kind, getAlphabeticallySortedKeys(r.StateStores))
	}

	resoures, err := stateStoreFunc(resource, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	values := map[string]renderers.ComputedValueReference{
		"stateStoreName": {
			Value: resource.ResourceName,
		},
	}
	secrets := map[string]renderers.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      resoures,
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}

func getAlphabeticallySortedKeys(store map[string]StateStoreFunc) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}
