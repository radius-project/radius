// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned RedisCache resource to version-agnostic datamodel.
func (src *RedisCacheResource) ConvertTo() (api.DataModelInterface, error) {
	secrets := datamodel.RedisSecrets{}
	if src.Properties.Secrets != nil {
		secrets = datamodel.RedisSecrets{
			ConnectionString: to.String(src.Properties.Secrets.ConnectionString),
			Password:         to.String(src.Properties.Secrets.Password),
		}
	}

	converted := &datamodel.RedisCache{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.RedisCacheProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			Resource:          to.String(src.Properties.Resource),
			Host:              to.String(src.Properties.Host),
			Port:              to.Int32(src.Properties.Port),
			Secrets:           secrets,
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCache resource.
func (dst *RedisCacheResource) ConvertFrom(src api.DataModelInterface) error {
	redis, ok := src.(*datamodel.RedisCache)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(redis.ID)
	dst.Name = to.StringPtr(redis.Name)
	dst.Type = to.StringPtr(redis.Type)
	dst.SystemData = fromSystemDataModel(redis.SystemData)
	dst.Location = to.StringPtr(redis.Location)
	dst.Tags = *to.StringMapPtr(redis.Tags)
	dst.Properties = &RedisCacheProperties{
		ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
		Environment:       to.StringPtr(redis.Properties.Environment),
		Application:       to.StringPtr(redis.Properties.Application),
		Resource:          to.StringPtr(redis.Properties.Resource),
		Host:              to.StringPtr(redis.Properties.Host),
		Port:              to.Int32Ptr(redis.Properties.Port),
	}
	if (redis.Properties.Secrets != datamodel.RedisSecrets{}) {
		dst.Properties.Secrets = &RedisCachePropertiesSecrets{
			ConnectionString: to.StringPtr(redis.Properties.Secrets.ConnectionString),
			Password:         to.StringPtr(redis.Properties.Secrets.Password),
		}
	}

	return nil
}
