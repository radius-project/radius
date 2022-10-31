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
				Application: to.String(src.Properties.GetMongoDatabaseProperties().Environment),
			},
			//ProvisioningState: toProvisioningStateDataModel(src.Properties.GetMongoDatabaseProperties().ProvisioningState),
		},
	}
	switch v := src.Properties.(type) {
	case *MongoDatabaseResourceProperties:
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
		//converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)
	case *ValuesMongoDatabaseRequestProperties:
		converted.Properties.MongoDatabaseValuesProperties = datamodel.MongoDatabaseValuesProperties{
			Host:     to.String(v.Host),
			Port:     to.Int32(v.Port),
			Database: to.String(v.Database),
		}
		if v.Secrets != nil {
			converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
				ConnectionString: to.String(v.Secrets.ConnectionString),
				Username:         to.String(v.Secrets.Username),
				Password:         to.String(v.Secrets.Password),
			}
		}
		//converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)
	/*case *MongoDatabaseRecipeProperties:
	converted.Properties.MongoDatabaseRecipeProperties = datamodel.MongoDatabaseRecipeProperties{
		Recipe: toRecipeDataModel(v.Recipe),
	}
	if v.Secrets != nil {
		converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
			ConnectionString: to.String(v.Secrets.ConnectionString),
			Username:         to.String(v.Secrets.Username),
			Password:         to.String(v.Secrets.Password),
		}
	}
	converted.Properties.Mode = toMongoDatabaseModeDataModel(src.Properties.GetMongoDatabaseProperties().Mode)*/
	default:
		return nil, errors.New("mode of Mongo Database is not specified")
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
	case datamodel.MongoDatabaseModeResource:
		converted := &MongoDatabaseResourceProperties{
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
		if (mongo.Properties.Secrets != datamodel.MongoDatabaseSecrets{}) {
			converted.Secrets = &MongoDatabaseSecrets{
				ConnectionString: to.StringPtr(mongo.Properties.Secrets.ConnectionString),
				Username:         to.StringPtr(mongo.Properties.Secrets.Username),
				Password:         to.StringPtr(mongo.Properties.Secrets.Password),
			}
		}
		dst.Properties = converted
	case datamodel.MongoDatabaseModeValues:
		converted := &MongoDatabaseValuesProperties{
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
		if (mongo.Properties.Secrets != datamodel.MongoDatabaseSecrets{}) {
			converted.Secrets = &MongoDatabaseSecrets{
				ConnectionString: to.StringPtr(mongo.Properties.Secrets.ConnectionString),
				Username:         to.StringPtr(mongo.Properties.Secrets.Username),
				Password:         to.StringPtr(mongo.Properties.Secrets.Password),
			}
		}
		dst.Properties = converted
	case datamodel.MongoDatabaseModeRecipe:
		/*converted := &MongoDatabaseRecipeProperties{
			Mode:   fromMongoDatabaseModeDataModel(mongo.Properties.Mode),
			Recipe: fromRecipeDataModel(mongo.Properties.Recipe),
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
			Environment:       to.StringPtr(mongo.Properties.Environment),
			Application:       to.StringPtr(mongo.Properties.Application),
		}
		if (mongo.Properties.Secrets != datamodel.MongoDatabaseSecrets{}) {
			converted.Secrets = &MongoDatabaseSecrets{
				ConnectionString: to.StringPtr(mongo.Properties.Secrets.ConnectionString),
				Username:         to.StringPtr(mongo.Properties.Secrets.Username),
				Password:         to.StringPtr(mongo.Properties.Secrets.Password),
			}
		}
		dst.Properties = converted*/
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

func fromMongoDatabaseModeDataModel(mode datamodel.MongoDatabaseMode) *MongoDatabasePropertiesMode {
	var convertedMode MongoDatabasePropertiesMode
	switch mode {
	case datamodel.MongoDatabaseModeResource:
		convertedMode = MongoDatabasePropertiesModeResource
	case datamodel.MongoDatabaseModeRecipe:
		convertedMode = MongoDatabasePropertiesModeRecipe
	case datamodel.MongoDatabaseModeValues:
		convertedMode = MongoDatabasePropertiesModeValues
	}
	return &convertedMode
}

func fromMongoDatabaseModeResponseDataModel(mode datamodel.MongoDatabaseMode) string {
	switch mode {
	case datamodel.MongoDatabaseModeResource:
		return "resource"
	case datamodel.MongoDatabaseModeRecipe:
		return "recipe"
	case datamodel.MongoDatabaseModeValues:
		return "values"
	}
	return ""
}
func toMongoDatabaseModeResponseDataModel(mode *string) datamodel.MongoDatabaseMode {
	switch mode {
	case to.StringPtr("resource"):
		return datamodel.MongoDatabaseModeResource
	case to.StringPtr("recipe"):
		return datamodel.MongoDatabaseModeRecipe
	case to.StringPtr("values"):
		return datamodel.MongoDatabaseModeValues
	}
	return ""
}

func toMongoDatabaseModeDataModel(mode *MongoDatabasePropertiesMode) datamodel.MongoDatabaseMode {
	var converted datamodel.MongoDatabaseMode
	switch *mode {
	case MongoDatabasePropertiesModeRecipe:
		converted = datamodel.MongoDatabaseModeResource
	case MongoDatabasePropertiesModeResource:
		converted = datamodel.MongoDatabaseModeRecipe
	case MongoDatabasePropertiesModeValues:
		converted = datamodel.MongoDatabaseModeValues
	}
	return converted
}
