// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/cosmosdbmongov1alpha3"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

// Most of the work is done by the cosmos renderer, we're just calling into it.
type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r AzureRenderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := MongoDBComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := renderers.RendererOutput{}
	converted := r.convertToCosmosDBMongo(properties)
	if converted.Managed {
		resources, err := cosmosdbmongov1alpha3.RenderManaged(resource.ResourceName, converted)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		output.Resources = append(output.Resources, resources...)
	} else {
		resources, err := cosmosdbmongov1alpha3.RenderUnmanaged(resource.ResourceName, converted)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		output.Resources = append(output.Resources, resources...)
	}

	computedValues, secretValues := cosmosdbmongov1alpha3.MakeSecretsAndValues(resource.ResourceName)
	output.ComputedValues = computedValues
	output.SecretValues = secretValues

	return output, nil
}

func (r AzureRenderer) convertToCosmosDBMongo(input MongoDBComponentProperties) cosmosdbmongov1alpha3.CosmosDBMongoComponentProperties {
	return cosmosdbmongov1alpha3.CosmosDBMongoComponentProperties{
		Managed:  input.Managed,
		Resource: input.Resource,
	}
}
