// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

type SecretStoreFunc = func(renderers.RendererResource) ([]outputresource.OutputResource, error)

var SupportedAzureSecretStoreKindValues = map[string]SecretStoreFunc{
	"generic": GetDaprSecretStoreAzureGeneric,
}

var SupportedKubernetesSecretStoreKindValues = map[string]SecretStoreFunc{
	"generic": GetDaprSecretStoreKubernetesGeneric,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	SecretStores map[string]SecretStoreFunc
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

	if r.SecretStores == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}

	secretStoreFunc := r.SecretStores[properties.Kind]
	if secretStoreFunc == nil {
		return renderers.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %s", properties.Kind, getAlphabeticallySortedKeys(r.SecretStores))
	}

	resoures, err := secretStoreFunc(resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	values := map[string]renderers.ComputedValueReference{
		"secretStoreName": {
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
