// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned RedisCache resource to version-agnostic datamodel.
func (src *RedisCacheResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetRedisCacheProperties().Environment),
				Application: to.String(src.Properties.GetRedisCacheProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetRedisCacheProperties().ProvisioningState),
		},
	}
	switch v := src.Properties.(type) {
	case *ResourceRedisCacheProperties:
		if v.Resource == nil {
			return &datamodel.RedisCache{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("resource is a required property for mode %q", datamodel.LinkModeResource))
		}
		converted.Properties.Mode = datamodel.LinkModeResource
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Username = to.String(v.Username)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.RedisCacheSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Password:         to.String(v.Secrets.Password),
			}
		}
	case *RecipeRedisCacheProperties:
		if v.Recipe == nil {
			return &datamodel.RedisCache{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("recipe is a required property for mode %q", datamodel.LinkModeRecipe))
		}
		converted.Properties.RedisRecipeProperties = datamodel.RedisRecipeProperties{
			Recipe: toRecipeDataModel(v.Recipe),
		}
		converted.Properties.Mode = datamodel.LinkModeRecipe
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Username = to.String(v.Username)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.RedisCacheSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Password:         to.String(v.Secrets.Password),
			}
		}
	case *ValuesRedisCacheProperties:
		if v.Host == nil || v.Port == nil {
			return &datamodel.RedisCache{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("host and port are required properties for mode %q", datamodel.LinkModeValues))
		}
		converted.Properties.Mode = datamodel.LinkModeValues
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Username = to.String(v.Username)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.RedisCacheSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Password:         to.String(v.Secrets.Password),
			}
		}
	default:
		return datamodel.RedisCache{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetRedisCacheProperties().Mode))
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
	switch redis.Properties.Mode {
	case datamodel.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceRedisCacheProperties{
			Mode:     &mode,
			Resource: to.StringPtr(redis.Properties.RedisResourceProperties.Resource),
			Host:     to.StringPtr(redis.Properties.Host),
			Port:     to.Int32Ptr(redis.Properties.Port),
			Username: to.StringPtr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
			Environment:       to.StringPtr(redis.Properties.Environment),
			Application:       to.StringPtr(redis.Properties.Application),
		}
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ResourceRedisCacheProperties{
			Mode:     &mode,
			Host:     to.StringPtr(redis.Properties.Host),
			Port:     to.Int32Ptr(redis.Properties.Port),
			Username: to.StringPtr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
			Environment:       to.StringPtr(redis.Properties.Environment),
			Application:       to.StringPtr(redis.Properties.Application),
		}
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeRedisCacheProperties{
			Mode:     &mode,
			Recipe:   fromRecipeDataModel(redis.Properties.Recipe),
			Host:     to.StringPtr(redis.Properties.Host),
			Port:     to.Int32Ptr(redis.Properties.Port),
			Username: to.StringPtr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.Properties.ProvisioningState),
			Environment:       to.StringPtr(redis.Properties.Environment),
			Application:       to.StringPtr(redis.Properties.Application),
		}
	default:
		return conv.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", redis.Properties.Mode))
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
