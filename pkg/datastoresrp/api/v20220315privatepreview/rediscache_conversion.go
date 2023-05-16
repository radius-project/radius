// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned RedisCache resource to version-agnostic datamodel.
func (src *RedisCacheResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetRedisCacheProperties().ProvisioningState),
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
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.GetRedisCacheProperties().Environment),
				Application: to.String(src.Properties.GetRedisCacheProperties().Application),
			},
		},
	}
	switch v := src.Properties.(type) {
	case *ResourceRedisCacheProperties:
		if v.Resource == nil {
			return &datamodel.RedisCache{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("resource is a required property for mode %q", linkrpdm.LinkModeResource))
		}
		converted.Properties.Mode = linkrpdm.LinkModeResource
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
			return &datamodel.RedisCache{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("recipe is a required property for mode %q", linkrpdm.LinkModeRecipe))
		}
		converted.Properties.RedisRecipeProperties = datamodel.RedisRecipeProperties{
			Recipe: toRecipeDataModel(v.Recipe),
		}
		converted.Properties.Mode = linkrpdm.LinkModeRecipe
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
			return &datamodel.RedisCache{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("host and port are required properties for mode %q", linkrpdm.LinkModeValues))
		}
		converted.Properties.Mode = linkrpdm.LinkModeValues
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
		return &datamodel.RedisCache{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetRedisCacheProperties().Mode))
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCache resource.
func (dst *RedisCacheResource) ConvertFrom(src v1.DataModelInterface) error {
	redis, ok := src.(*datamodel.RedisCache)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(redis.ID)
	dst.Name = to.Ptr(redis.Name)
	dst.Type = to.Ptr(redis.Type)
	dst.SystemData = fromSystemDataModel(redis.SystemData)
	dst.Location = to.Ptr(redis.Location)
	dst.Tags = *to.StringMapPtr(redis.Tags)
	switch redis.Properties.Mode {
	case linkrpdm.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceRedisCacheProperties{
			Mode:     &mode,
			Resource: to.Ptr(redis.Properties.RedisResourceProperties.Resource),
			Host:     to.Ptr(redis.Properties.Host),
			Port:     to.Ptr(redis.Properties.Port),
			Username: to.Ptr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(redis.Properties.Environment),
			Application:       to.Ptr(redis.Properties.Application),
		}
	case linkrpdm.LinkModeValues:
		mode := "values"
		dst.Properties = &ResourceRedisCacheProperties{
			Mode:     &mode,
			Host:     to.Ptr(redis.Properties.Host),
			Port:     to.Ptr(redis.Properties.Port),
			Username: to.Ptr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(redis.Properties.Environment),
			Application:       to.Ptr(redis.Properties.Application),
		}
	case linkrpdm.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeRedisCacheProperties{
			Mode:     &mode,
			Recipe:   fromRecipeDataModel(redis.Properties.Recipe),
			Host:     to.Ptr(redis.Properties.Host),
			Port:     to.Ptr(redis.Properties.Port),
			Username: to.Ptr(redis.Properties.Username),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(redis.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(redis.Properties.Environment),
			Application:       to.Ptr(redis.Properties.Application),
		}
	default:
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", redis.Properties.Mode))
	}
	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RedisCacheSecrets instance.
func (dst *RedisCacheSecrets) ConvertFrom(src v1.DataModelInterface) error {
	redisSecrets, ok := src.(*datamodel.RedisCacheSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.Ptr(redisSecrets.ConnectionString)
	dst.Password = to.Ptr(redisSecrets.Password)

	return nil
}

// ConvertTo converts from the versioned RedisCacheSecrets instance to version-agnostic datamodel.
func (src *RedisCacheSecrets) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RedisCacheSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Password:         to.String(src.Password),
	}
	return converted, nil
}
