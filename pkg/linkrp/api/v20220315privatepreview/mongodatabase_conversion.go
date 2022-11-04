// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned MongoDatabase resource to version-agnostic datamodel.
func (src *MongoDatabaseResource) ConvertTo() (conv.DataModelInterface, error) {
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
				UpdatedAPIVersion: Version,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetMongoDatabaseProperties().Environment),
				Application: to.String(src.Properties.GetMongoDatabaseProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetMongoDatabaseProperties().ProvisioningState),
		},
	}
	switch v := src.Properties.(type) {
	case *ResourceMongoDatabaseProperties:
		if v.Resource == nil {
			return &datamodel.MongoDatabase{}, conv.NewClientErrInvalidRequest("resource is a required properties")
		}
		converted.Properties.MongoDatabaseResourceProperties = datamodel.MongoDatabaseResourceProperties{
			Resource: to.String(v.Resource),
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
		converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)
	case *ValuesMongoDatabaseProperties:
		if v.Host == nil || v.Port == nil {
			return &datamodel.MongoDatabase{}, conv.NewClientErrInvalidRequest("host/port are required properties")
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
		converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)
	case *RecipeMongoDatabaseProperties:
		if v.Recipe == nil {
			return &datamodel.MongoDatabase{}, conv.NewClientErrInvalidRequest("recipe is a required properties")
		}
		converted.Properties.MongoDatabaseRecipeProperties = datamodel.MongoDatabaseRecipeProperties{
			Recipe: toRecipeDataModel(v.Recipe),
		}
		converted.Properties.Host = to.String(v.Host)
		converted.Properties.Port = to.Int32(v.Port)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Username:         to.String(v.Secrets.Username),
				Password:         to.String(v.Secrets.Password),
			}
		}
	default:
		return datamodel.MongoDatabase{}, conv.NewClientErrInvalidRequest("Invalid Mode for mongo database")
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabase resource.
func (dst *MongoDatabaseResource) ConvertFrom(src conv.DataModelInterface) error {
	mongo, ok := src.(*datamodel.MongoDatabase)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(mongo.ID)
	dst.Name = to.StringPtr(mongo.Name)
	dst.Type = to.StringPtr(mongo.Type)
	dst.SystemData = fromSystemDataModel(mongo.SystemData)
	dst.Location = to.StringPtr(mongo.Location)
	dst.Tags = *to.StringMapPtr(mongo.Tags)

	switch mongo.Properties.Mode {
	case datamodel.LinkModeResource:
		converted := &ResourceMongoDatabaseProperties{
			Mode:     fromMongoDatabaseModeDataModel(mongo.Properties.Mode),
			Resource: to.StringPtr(mongo.Properties.MongoDatabaseResourceProperties.Resource),
			Host:     to.StringPtr(mongo.Properties.Host),
			Port:     to.Int32Ptr(mongo.Properties.Port),
			Database: to.StringPtr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
			Environment:       to.StringPtr(mongo.Properties.Environment),
			Application:       to.StringPtr(mongo.Properties.Application),
		}
		dst.Properties = converted
	case datamodel.LinkModeValues:
		converted := &ValuesMongoDatabaseProperties{
			Mode:     fromMongoDatabaseModeDataModel(mongo.Properties.Mode),
			Host:     to.StringPtr(mongo.Properties.Host),
			Port:     to.Int32Ptr(mongo.Properties.Port),
			Database: to.StringPtr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
			Environment:       to.StringPtr(mongo.Properties.Environment),
			Application:       to.StringPtr(mongo.Properties.Application),
		}
		dst.Properties = converted
	case datamodel.LinkModeRecipe:
		converted := &RecipeMongoDatabaseProperties{
			Mode:     fromMongoDatabaseModeDataModel(mongo.Properties.Mode),
			Recipe:   fromRecipeDataModel(mongo.Properties.Recipe),
			Host:     to.StringPtr(mongo.Properties.Host),
			Port:     to.Int32Ptr(mongo.Properties.Port),
			Database: to.StringPtr(mongo.Properties.Database),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
			Environment:       to.StringPtr(mongo.Properties.Environment),
			Application:       to.StringPtr(mongo.Properties.Application),
		}
		dst.Properties = converted
	default:
		return errors.New("mode of Mongo Database is not specified")
	}

	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabaseSecrets instance.
func (dst *MongoDatabaseSecrets) ConvertFrom(src conv.DataModelInterface) error {
	mongoSecrets, ok := src.(*datamodel.MongoDatabaseSecrets)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.StringPtr(mongoSecrets.ConnectionString)
	dst.Username = to.StringPtr(mongoSecrets.Username)
	dst.Password = to.StringPtr(mongoSecrets.Password)

	return nil
}

// ConvertTo converts from the versioned MongoDatabaseSecrets instance to version-agnostic datamodel.
func (src *MongoDatabaseSecrets) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.MongoDatabaseSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Username:         to.String(src.Username),
		Password:         to.String(src.Password),
	}
	return converted, nil
}

func fromMongoDatabaseModeDataModel(mode datamodel.LinkMode) *MongoDatabasePropertiesMode {
	var convertedMode MongoDatabasePropertiesMode
	switch mode {
	case datamodel.LinkModeResource:
		convertedMode = MongoDatabasePropertiesModeResource
	case datamodel.LinkModeRecipe:
		convertedMode = MongoDatabasePropertiesModeRecipe
	case datamodel.LinkModeValues:
		convertedMode = MongoDatabasePropertiesModeValues
	}
	return &convertedMode
}

func toMongoDatabaseModeDataModel(mode *MongoDatabasePropertiesMode) datamodel.LinkMode {
	var converted datamodel.LinkMode
	switch *mode {
	case MongoDatabasePropertiesModeRecipe:
		converted = datamodel.LinkModeRecipe
	case MongoDatabasePropertiesModeResource:
		converted = datamodel.LinkModeResource
	case MongoDatabasePropertiesModeValues:
		converted = datamodel.LinkModeValues
	}
	return converted
}
