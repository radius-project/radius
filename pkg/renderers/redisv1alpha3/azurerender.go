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

const (
	ConnectionStringValue = "connectionString"
	PasswordValue         = "password"
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

	redisCacheOutputResource, err := RenderResource(resource.ResourceName, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if redisCacheOutputResource != nil {
		outputResources = append(outputResources, *redisCacheOutputResource)
	}

	computedValues, secretValues := MakeSecretsAndValues(resource.ResourceName, properties)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderResource(resourceName string, properties radclient.RedisCacheResourceProperties) (*outputresource.OutputResource, error) {
	if properties.Secrets != nil {
		// When the user-specified secret is present, this is the usecase where the user is running
		// their own custom Redis instance (using a container, or hosted elsewhere).
		//
		// In that case we don't have an OutputResaource, only Computed and Secret values.
		return nil, nil
	}
	if properties.Resource == nil || *properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForResource
	}

	redisResourceID, err := renderers.ValidateResourceID(*properties.Resource, RedisResourceType, "Redis Cache")
	if err != nil {
		return nil, err
	}

	redisCacheOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureRedis,
		ResourceKind: resourcekinds.AzureRedis,
		Resource: map[string]string{
			handlers.RedisResourceIdKey: redisResourceID.ID,
			handlers.RedisNameKey:       redisResourceID.Name(),
		},
	}
	return &redisCacheOutputResource, nil
}

func MakeSecretsAndValues(name string, properties radclient.RedisCacheResourceProperties) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: to.String(properties.Host),
		},
		"port": {
			Value: strconv.Itoa(int(to.Int32(properties.Port))),
		},
	}
	if properties.Secrets == nil {
		return computedValues, nil
	}
	// Currently user-specfied secrets are stored in the `secrets` property of the resource, and
	// thus serialized to our database.
	//
	// TODO(#1767): We need to store these in a secret store.
	secretValues := map[string]renderers.SecretValueReference{}
	secretValues[ConnectionStringValue] = renderers.SecretValueReference{Value: *properties.Secrets.ConnectionString}
	secretValues[PasswordValue] = renderers.SecretValueReference{Value: *properties.Secrets.Password}
	return computedValues, secretValues
}
