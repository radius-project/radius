// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned RedisCache resource to version-agnostic datamodel.
func (src *RedisCacheResource) ConvertTo() (conv.DataModelInterface, error) {
	recipe := v1.Recipe{}
	if src.Properties.Recipe != nil {
		recipe = v1.Recipe{
			Name:       to.String(src.Properties.Recipe.Name),
			Parameters: src.Properties.Recipe.Parameters,
		}
	}
	secrets := datamodel.RedisCacheSecrets{}
	if src.Properties.Secrets != nil {
		secrets = datamodel.RedisCacheSecrets{
			ConnectionString: to.String(src.Properties.Secrets.ConnectionString),
			Password:         to.String(src.Properties.Secrets.Password),
		}
	}
	converted := &datamodel.RedisCache{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.RedisCacheProperties{
			RedisCacheResponseProperties: datamodel.RedisCacheResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Environment: to.String(src.Properties.Environment),
					Application: to.String(src.Properties.Application),
				},
				ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
				Resource:          to.String(src.Properties.Resource),
				Host:              to.String(src.Properties.Host),
				Port:              to.Int32(src.Properties.Port),
				Username:          to.String(src.Properties.Username),
				Recipe:            recipe,
			},
			Secrets: secrets,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertTo converts from the versioned RedisCacheResponse resource to version-agnostic datamodel.
func (src *RedisCacheResponseResource) ConvertTo() (conv.DataModelInterface, error) {
	recipe := v1.Recipe{}
	if src.Properties.Recipe != nil {
		recipe = v1.Recipe{
			Name:       to.String(src.Properties.Recipe.Name),
			Parameters: src.Properties.Recipe.Parameters,
		}
	}
	converted := &datamodel.RedisCacheResponse{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.RedisCacheResponseProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Resource:          to.String(src.Properties.Resource),
			Host:              to.String(src.Properties.Host),
			Port:              to.Int32(src.Properties.Port),
			Username:          to.String(src.Properties.Username),
			Recipe:            recipe,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCache resource.
func (dst *RedisCacheResource) ConvertFrom(src conv.DataModelInterface) error {
	redis, ok := src.(*datamodel.RedisCache)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(redis.ID)
	dst.Name = to.StringPtr(redis.Name)
	dst.Type = to.StringPtr(redis.Type)
	dst.SystemData = fromSystemDataModel(redis.SystemData)
	dst.Location = to.StringPtr(redis.Location)
	dst.Tags = *to.StringMapPtr(redis.Tags)
	dst.Properties = &RedisCacheProperties{
		Status: &ResourceStatus{
			OutputResources: v1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
		Environment:       to.StringPtr(redis.Properties.Environment),
		Application:       to.StringPtr(redis.Properties.Application),
		Resource:          to.StringPtr(redis.Properties.Resource),
		Host:              to.StringPtr(redis.Properties.Host),
		Port:              to.Int32Ptr(redis.Properties.Port),
		Username:          to.StringPtr(redis.Properties.Username),
		Recipe: &Recipe{
			Name:       to.StringPtr(redis.Properties.Recipe.Name),
			Parameters: v1.BuildRecipePramaeter(redis.Properties.Recipe.Parameters),
		},
	}
	if (redis.Properties.Secrets != datamodel.RedisCacheSecrets{}) {
		dst.Properties.Secrets = &RedisCacheSecrets{
			ConnectionString: to.StringPtr(redis.Properties.Secrets.ConnectionString),
			Password:         to.StringPtr(redis.Properties.Secrets.Password),
		}
	}

	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCache response resource.
func (dst *RedisCacheResponseResource) ConvertFrom(src conv.DataModelInterface) error {
	redis, ok := src.(*datamodel.RedisCacheResponse)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(redis.ID)
	dst.Name = to.StringPtr(redis.Name)
	dst.Type = to.StringPtr(redis.Type)
	dst.SystemData = fromSystemDataModel(redis.SystemData)
	dst.Location = to.StringPtr(redis.Location)
	dst.Tags = *to.StringMapPtr(redis.Tags)
	dst.Properties = &RedisCacheResponseProperties{
		Status: &ResourceStatus{
			OutputResources: v1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
		Environment:       to.StringPtr(redis.Properties.Environment),
		Application:       to.StringPtr(redis.Properties.Application),
		Resource:          to.StringPtr(redis.Properties.Resource),
		Host:              to.StringPtr(redis.Properties.Host),
		Port:              to.Int32Ptr(redis.Properties.Port),
		Username:          to.StringPtr(redis.Properties.Username),
		Recipe: &Recipe{
			Name:       &redis.Properties.Recipe.Name,
			Parameters: redis.Properties.Recipe.Parameters,
		},
	}
	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCacheSecrets instance.
func (dst *RedisCacheSecrets) ConvertFrom(src conv.DataModelInterface) error {
	redisSecrets, ok := src.(*datamodel.RedisCacheSecrets)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.StringPtr(redisSecrets.ConnectionString)
	dst.Password = to.StringPtr(redisSecrets.Password)

	return nil
}

// ConvertTo converts from the versioned RedisCacheSecrets instance to version-agnostic datamodel.
func (src *RedisCacheSecrets) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.RedisCacheSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Password:         to.String(src.Password),
	}
	return converted, nil
}
