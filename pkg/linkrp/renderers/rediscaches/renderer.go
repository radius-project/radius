// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/redis/mgmt/redis"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
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

	if resource.Properties.Resource == "" {
		return renderers.RendererOutput{
			Resources:      []outputresource.OutputResource{},
			ComputedValues: computedValues,
			SecretValues:   secretValues,
		}, nil
	} else {
		// Source resource identifier is provided. Currently only Azure resources are expected with non empty resource id
		rendererOutput, err := renderAzureResource(properties, secretValues, computedValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	}
}

func renderAzureResource(properties datamodel.RedisCacheProperties, secretValues map[string]rp.SecretValueReference, computedValues map[string]renderers.ComputedValueReference) (renderers.RendererOutput, error) {
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

	if _, ok := computedValues[renderers.Host]; !ok {
		computedValues[renderers.Host] = renderers.ComputedValueReference{
			LocalID:     outputresource.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName", // https://learn.microsoft.com/en-us/rest/api/redis/redis/get
		}
	}

	if _, ok := computedValues[renderers.Port]; !ok {
		computedValues[renderers.Port] = renderers.ComputedValueReference{
			LocalID:     outputresource.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort", // https://learn.microsoft.com/en-us/rest/api/redis/redis/get
		}
	}

	if _, ok := secretValues[renderers.PasswordStringHolder]; !ok {
		secretValues[renderers.PasswordStringHolder] = rp.SecretValueReference{
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		}
	}

	if _, ok := secretValues[renderers.ConnectionStringValue]; !ok {
		secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureRedis,
			},
		}
	}

	// Build output resources
	redisCacheOutputResource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureRedis,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRedis,
			Provider: resourcemodel.ProviderAzure,
		},
	}
	redisCacheOutputResource.Identity = resourcemodel.NewARMIdentity(&redisCacheOutputResource.ResourceType, redisCacheID.String(), clients.GetAPIVersionFromUserAgent(redis.UserAgent()))

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{redisCacheOutputResource},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func getProvidedSecretValues(properties datamodel.RedisCacheProperties) map[string]rp.SecretValueReference {
	secretValues := map[string]rp.SecretValueReference{}
	if !properties.Secrets.IsEmpty() {
		if properties.Secrets.Password != "" {
			secretValues[renderers.PasswordStringHolder] = rp.SecretValueReference{Value: properties.Secrets.Password}
		}
		if properties.Secrets.ConnectionString != "" {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: properties.Secrets.ConnectionString}
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
