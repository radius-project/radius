// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.RedisCache)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretValues := getProvidedSecretValues(properties)

	if resource.Properties.Resource == "" {
		return renderers.RendererOutput{
			Resources: []outputresource.OutputResource{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				renderers.Host: {
					Value: resource.Properties.Host,
				},
				renderers.Port: {
					Value: resource.Properties.Port,
				},
				renderers.UsernameStringValue: {
					Value: "",
				},
			},
			SecretValues: secretValues,
		}, nil
	} else {
		// Source resource identifier is provided, currently only Azure resources are expected with non empty resource id
		rendererOutput, err := RenderAzureResource(properties, secretValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	}
}

func RenderAzureResource(properties datamodel.RedisCacheProperties, secretValues map[string]rp.SecretValueReference) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this connector
	redisCacheID, err := resources.Parse(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, errors.New("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Redis Cache resource type
	err = redisCacheID.ValidateResourceType(RedisResourceType)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("the 'resource' field must refer to a %s", "Redis Cache")
	}

	computedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			Value: properties.Host,
		},
		renderers.Port: {
			Value: properties.Port,
		},
		renderers.UsernameStringValue: {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisUsernameKey,
		},
	}

	// Populate connection string reference if a value isn't provided
	if properties.Secrets.IsEmpty() || properties.Secrets.ConnectionString == "" {
		secretValues = map[string]rp.SecretValueReference{
			renderers.PasswordStringHolder: {
				LocalID:       outputresource.LocalIDAzureRedis,
				Action:        "listKeys",
				ValueSelector: "/primaryKey",
			},
		}
	}

	// Build output resources
	redisCacheOutputResource := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureRedis,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRedis,
				Provider: providers.ProviderAzure,
			},
			Resource: map[string]string{
				handlers.RedisResourceIdKey: redisCacheID.String(),
				handlers.RedisNameKey:       redisCacheID.Name(),
			},
		},
	}

	return renderers.RendererOutput{
		Resources:      redisCacheOutputResource,
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
