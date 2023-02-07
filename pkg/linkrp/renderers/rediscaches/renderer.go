// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.RedisCache)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretValues := getProvidedSecretValues(properties)
	computedValues := getProvidedComputedValues(properties)

	_, err := renderers.ValidateApplicationID(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	switch resource.Properties.Mode {

	case datamodel.LinkModeRecipe:
		rendererOutput, err := renderAzureRecipe(resource, options, secretValues, computedValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		return rendererOutput, nil
	case datamodel.LinkModeResource:
		// Source resource identifier is provided. Currently only Azure resources are expected with non empty resource id
		rendererOutput, err := renderAzureResource(properties, secretValues, computedValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		return rendererOutput, nil
	case datamodel.LinkModeValues:
		return renderers.RendererOutput{
			Resources:      []rpv1.OutputResource{},
			ComputedValues: computedValues,
			SecretValues:   secretValues,
		}, nil
	default:
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("unsupported mode %s", resource.Properties.Mode))
	}

}

func renderAzureRecipe(resource *datamodel.RedisCache, options renderers.RenderOptions, secretValues map[string]rpv1.SecretValueReference, computedValues map[string]renderers.ComputedValueReference) (renderers.RendererOutput, error) {
	if options.RecipeProperties.LinkType != resource.ResourceTypeName() {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("link type %q of provided recipe %q is incompatible with %q resource type. Recipe link type must match link resource type.",
			options.RecipeProperties.LinkType, options.RecipeProperties.Name, linkrp.RedisCachesResourceType))
	}

	recipeData := linkrp.RecipeData{
		RecipeProperties: options.RecipeProperties,
		APIVersion:       clientv2.RedisManagementClientAPIVersion,
	}

	// Build computedValues reference
	buildComputedValuesReference(computedValues)
	// Build secretValue reference
	buildSecretValueReference(secretValues)

	// Build output resources
	redisCacheOutputResource := buildOutputResource()
	redisCacheOutputResource.ProviderResourceType = azresources.CacheRedis
	// Set the RadiusManaged to true for resources deployed by recipe
	redisCacheOutputResource.RadiusManaged = to.Ptr(true)

	return renderers.RendererOutput{
		Resources:            []rpv1.OutputResource{redisCacheOutputResource},
		ComputedValues:       computedValues,
		SecretValues:         secretValues,
		RecipeData:           recipeData,
		EnvironmentProviders: options.EnvironmentProviders,
	}, nil
}

func renderAzureResource(properties datamodel.RedisCacheProperties, secretValues map[string]rpv1.SecretValueReference, computedValues map[string]renderers.ComputedValueReference) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this link
	redisCacheID, err := resources.ParseResource(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Redis Cache resource type
	err = redisCacheID.ValidateResourceType(RedisResourceType)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must refer to an Azure Redis Cache")
	}

	// Build computedValues reference
	buildComputedValuesReference(computedValues)
	// Build secretValue reference
	buildSecretValueReference(secretValues)

	// Build output resources
	redisCacheOutputResource := buildOutputResource()
	redisCacheOutputResource.Identity = resourcemodel.NewARMIdentity(&redisCacheOutputResource.ResourceType, redisCacheID.String(), clientv2.RedisManagementClientAPIVersion)

	return renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{redisCacheOutputResource},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func getProvidedSecretValues(properties datamodel.RedisCacheProperties) map[string]rpv1.SecretValueReference {
	secretValues := map[string]rpv1.SecretValueReference{}
	if !properties.Secrets.IsEmpty() {
		if properties.Secrets.Password != "" {
			secretValues[renderers.PasswordStringHolder] = rpv1.SecretValueReference{Value: properties.Secrets.Password}
		}
		if properties.Secrets.ConnectionString != "" {
			secretValues[renderers.ConnectionStringValue] = rpv1.SecretValueReference{Value: properties.Secrets.ConnectionString}
		}
	}

	return secretValues
}

func getProvidedComputedValues(properties datamodel.RedisCacheProperties) map[string]renderers.ComputedValueReference {
	computedValues := map[string]renderers.ComputedValueReference{}
	if properties.Host != "" {
		computedValues[renderers.Host] = renderers.ComputedValueReference{Value: properties.Host}
	}
	if properties.Port != 0 {
		computedValues[renderers.Port] = renderers.ComputedValueReference{Value: properties.Port}
	}

	return computedValues
}

func buildSecretValueReference(secretValues map[string]rpv1.SecretValueReference) map[string]rpv1.SecretValueReference {
	if _, ok := secretValues[renderers.PasswordStringHolder]; !ok {
		secretValues[renderers.PasswordStringHolder] = rpv1.SecretValueReference{
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		}
	}

	if _, ok := secretValues[renderers.ConnectionStringValue]; !ok {
		secretValues[renderers.ConnectionStringValue] = rpv1.SecretValueReference{
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureRedis,
			},
		}
	}
	return secretValues
}

func buildComputedValuesReference(computedValues map[string]renderers.ComputedValueReference) {
	if _, ok := computedValues[renderers.Host]; !ok {
		computedValues[renderers.Host] = renderers.ComputedValueReference{
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName", // https://learn.microsoft.com/en-us/rest/api/redis/redis/get
		}
	}

	if _, ok := computedValues[renderers.Port]; !ok {
		computedValues[renderers.Port] = renderers.ComputedValueReference{
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort", // https://learn.microsoft.com/en-us/rest/api/redis/redis/get
		}
	}
}

func buildOutputResource() rpv1.OutputResource {
	return rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureRedis,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRedis,
			Provider: resourcemodel.ProviderAzure,
		},
	}
}
