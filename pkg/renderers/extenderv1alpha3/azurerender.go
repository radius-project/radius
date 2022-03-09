// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenderv1alpha3

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

const (
	ResourceType = "Extender"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
	Arm armauth.ArmConfig
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.ExtenderProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues, secretValues := MakeSecretsAndValues(options.Resource.ResourceName, properties)

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func MakeSecretsAndValues(name string, properties radclient.ExtenderProperties) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValueReferences := map[string]renderers.ComputedValueReference{}
	for k, v := range properties.AdditionalProperties {
		computedValueReferences[k] = renderers.ComputedValueReference{
			Value: v,
		}
	}
	if properties.Secrets == nil {
		return computedValueReferences, nil
	}

	// Create secret value references to point to the secret output resources created above
	secretValues := map[string]renderers.SecretValueReference{}
	for k, v := range properties.Secrets {
		secretValues[k] = renderers.SecretValueReference{
			Value: v.(string),
		}
	}

	return computedValueReferences, secretValues
}
