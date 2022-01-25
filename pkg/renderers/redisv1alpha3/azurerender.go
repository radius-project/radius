// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"strconv"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.RedisCacheResourceProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources := []outputresource.OutputResource{}
	if properties.Managed != nil && *properties.Managed {
		redisCacheOutputResource := RenderManaged(resource.ResourceName, properties)

		outputResources = append(outputResources, redisCacheOutputResource)
	} else {
		redisCacheOutputResource, err := RenderUnmanaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		if redisCacheOutputResource != nil {
			outputResources = append(outputResources, *redisCacheOutputResource)
		}
	}

	computedValues, secretValues := MakeSecretsAndValues(resource.ResourceName, properties)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderManaged(resourceName string, properties radclient.RedisCacheResourceProperties) outputresource.OutputResource {
	redisCacheOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureRedis,
		ResourceKind: resourcekinds.AzureRedis,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:    "true",
			handlers.RedisBaseName: resourceName,
		},
	}

	return redisCacheOutputResource
}

func RenderUnmanaged(resourceName string, properties radclient.RedisCacheResourceProperties) (*outputresource.OutputResource, error) {
	if properties.Secrets != nil {
		return nil, nil
	}
	if properties.Resource == nil || *properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	redisResourceID, err := renderers.ValidateResourceID(*properties.Resource, RedisResourceType, "Redis Cache")
	if err != nil {
		return nil, err
	}

	redisCacheOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureRedis,
		ResourceKind: resourcekinds.AzureRedis,
		Managed:      false,
		Resource: map[string]string{
			handlers.ManagedKey:         "false",
			handlers.RedisResourceIdKey: redisResourceID.ID,
			handlers.RedisNameKey:       redisResourceID.Name(),
		},
	}
	return &redisCacheOutputResource, nil
}

func MakeSecretsAndValues(name string, properties radclient.RedisCacheResourceProperties) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	if properties.Secrets == nil {
		computedValues := map[string]renderers.ComputedValueReference{
			"host": {
				LocalID:           outputresource.LocalIDAzureRedis,
				PropertyReference: handlers.RedisHostKey,
			},
			"port": {
				LocalID:           outputresource.LocalIDAzureRedis,
				PropertyReference: handlers.RedisPortKey,
			},
			"username": {
				LocalID:           outputresource.LocalIDAzureRedis,
				PropertyReference: handlers.RedisUsernameKey,
			},
		}

		secretValues := map[string]renderers.SecretValueReference{
			"password": {
				LocalID:       outputresource.LocalIDAzureRedis,
				Action:        "listKeys",
				ValueSelector: "/primaryKey",
			},
		}

		return computedValues, secretValues
	}
	// Currently user-specfied secrets are stored in the `secrets` property of the resource, and
	// thus serialized to our database.
	//
	// TODO(#1767): We need to store these in a secret store.
	return map[string]renderers.ComputedValueReference{
		"host": {
			Value: to.String(properties.Host),
		},
		"port": {
			Value: strconv.Itoa(int(to.Int32(properties.Port))),
		},
		"username": {
			Value: "",
		},
		"password": {
			Value: to.String(properties.Secrets.Password),
		},
		"connectionString": {
			Value: to.String(properties.Secrets.ConnectionString),
		},
	}, nil
}
