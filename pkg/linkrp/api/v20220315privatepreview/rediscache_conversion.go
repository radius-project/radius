/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v20220315privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned RedisCache resource to version-agnostic datamodel.
func (src *RedisCacheResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
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
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
		},
	}
	v := src.Properties

	converted.Properties.ResourceProvisioning = toResourceProvisiongDataModel(v.ResourceProvisioning)
	var found bool
	for _, k := range PossibleResourceProvisioningValues() {
		if ResourceProvisioning(converted.Properties.ResourceProvisioning) == k {
			found = true
			break
		}
	}
	if !found {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
	}

	converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
	converted.Properties.Resources = toResourcesDataModel(v.Resources)
	converted.Properties.Host = to.String(v.Host)
	converted.Properties.Port = to.Int32(v.Port)
	converted.Properties.Username = to.String(v.Username)
	if v.Secrets != nil {
		converted.Properties.Secrets = datamodel.RedisCacheSecrets{
			ConnectionString: to.String(v.Secrets.ConnectionString),
			Password:         to.String(v.Secrets.Password),
		}
	}
	manualInputs := src.verifyManualInputs()
	if manualInputs != nil {
		return nil, manualInputs
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

	dst.Properties = &RedisCacheProperties{
		Recipe:               fromRecipeDataModel(redis.Properties.Recipe),
		ResourceProvisioning: fromResourceProvisioningDataModel(redis.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(redis.Properties.Resources),
		Host:                 to.Ptr(redis.Properties.Host),
		Port:                 to.Ptr(redis.Properties.Port),
		Username:             to.Ptr(redis.Properties.Username),
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(redis.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(redis.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(redis.Properties.Environment),
		Application:       to.Ptr(redis.Properties.Application),
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

func (src *RedisCacheResource) verifyManualInputs() error {
	properties := src.Properties
	if properties.ResourceProvisioning != nil && *properties.ResourceProvisioning == ResourceProvisioning(linkrp.ResourceProvisioningManual) {
		if properties.Host == nil || properties.Port == nil {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("host and port are required when resourceProvisioning is %s", ResourceProvisioningManual)}
		}
	}
	return nil
}
