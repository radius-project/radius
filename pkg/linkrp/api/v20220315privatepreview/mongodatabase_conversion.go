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
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned MongoDatabase resource to version-agnostic datamodel.
func (src *MongoDatabaseResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetMongoDatabaseProperties().ProvisioningState),
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.GetMongoDatabaseProperties().Environment),
				Application: to.String(src.Properties.GetMongoDatabaseProperties().Application),
			},
		},
	}
	switch v := src.Properties.(type) {
	case *ResourceMongoDatabaseProperties:
		if v.Resource == nil {
			return &datamodel.MongoDatabase{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("resource is a required property for mode %q", datamodel.LinkModeResource))
		}
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Database = to.String(v.Database)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Username:         to.String(v.Secrets.Username),
				Password:         to.String(v.Secrets.Password),
			}
		}
		converted.Properties.Mode = datamodel.LinkModeResource
	case *ValuesMongoDatabaseProperties:
		if v.Host == nil || v.Port == nil {
			return &datamodel.MongoDatabase{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("host and port are required properties for mode %q", datamodel.LinkModeValues))
		}
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Database = to.String(v.Database)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Username:         to.String(v.Secrets.Username),
				Password:         to.String(v.Secrets.Password),
			}
		}
		converted.Properties.Mode = datamodel.LinkModeValues
	case *RecipeMongoDatabaseProperties:
		if v.Recipe == nil {
			return &datamodel.MongoDatabase{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("recipe is a required property for mode %q", datamodel.LinkModeRecipe))
		}
		converted.Properties.MongoDatabaseRecipeProperties = datamodel.MongoDatabaseRecipeProperties{
			Recipe: toRecipeDataModel(v.Recipe),
		}
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Mode = datamodel.LinkModeRecipe
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Username:         to.String(v.Secrets.Username),
				Password:         to.String(v.Secrets.Password),
			}
		}
	default:
		return &datamodel.MongoDatabase{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetMongoDatabaseProperties().Mode))
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabase resource.
func (dst *MongoDatabaseResource) ConvertFrom(src v1.DataModelInterface) error {
	mongo, ok := src.(*datamodel.MongoDatabase)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(mongo.ID)
	dst.Name = to.Ptr(mongo.Name)
	dst.Type = to.Ptr(mongo.Type)
	dst.SystemData = fromSystemDataModel(mongo.SystemData)
	dst.Location = to.Ptr(mongo.Location)
	dst.Tags = *to.StringMapPtr(mongo.Tags)

	switch mongo.Properties.Mode {
	case datamodel.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceMongoDatabaseProperties{
			Mode:     &mode,
			Resource: to.Ptr(mongo.Properties.MongoDatabaseResourceProperties.Resource),
			Host:     to.Ptr(mongo.Properties.Host),
			Port:     to.Ptr(mongo.Properties.Port),
			Database: to.Ptr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(mongo.Properties.Environment),
			Application:       to.Ptr(mongo.Properties.Application),
		}
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesMongoDatabaseProperties{
			Mode:     &mode,
			Host:     to.Ptr(mongo.Properties.Host),
			Port:     to.Ptr(mongo.Properties.Port),
			Database: to.Ptr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(mongo.Properties.Environment),
			Application:       to.Ptr(mongo.Properties.Application),
		}
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeMongoDatabaseProperties{
			Mode:     &mode,
			Recipe:   fromRecipeDataModel(mongo.Properties.Recipe),
			Host:     to.Ptr(mongo.Properties.Host),
			Port:     to.Ptr(mongo.Properties.Port),
			Database: to.Ptr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(mongo.Properties.Environment),
			Application:       to.Ptr(mongo.Properties.Application),
		}
	default:
		return fmt.Errorf("Unsupported mode %s", mongo.Properties.Mode)
	}

	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabaseSecrets instance.
func (dst *MongoDatabaseSecrets) ConvertFrom(src v1.DataModelInterface) error {
	mongoSecrets, ok := src.(*datamodel.MongoDatabaseSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.Ptr(mongoSecrets.ConnectionString)
	dst.Username = to.Ptr(mongoSecrets.Username)
	dst.Password = to.Ptr(mongoSecrets.Password)

	return nil
}

// ConvertTo converts from the versioned MongoDatabaseSecrets instance to version-agnostic datamodel.
func (src *MongoDatabaseSecrets) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.MongoDatabaseSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Username:         to.String(src.Username),
		Password:         to.String(src.Password),
	}
	return converted, nil
}
