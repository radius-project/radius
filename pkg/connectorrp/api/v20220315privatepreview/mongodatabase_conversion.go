// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned MongoDatabaseResponse resource to version-agnostic datamodel.
func (src *MongoDatabaseResponseResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.MongoDatabaseResponse{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.MongoDatabaseResponseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetMongoDatabaseResponseProperties().Environment),
				Application: to.String(src.Properties.GetMongoDatabaseResponseProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetMongoDatabaseResponseProperties().ProvisioningState),
			Resource:          to.String(src.Properties.GetMongoDatabaseResponseProperties().Resource),
			Host:              to.String(src.Properties.GetMongoDatabaseResponseProperties().Host),
			Port:              to.Int32(src.Properties.GetMongoDatabaseResponseProperties().Port),
			Database:          to.String(src.Properties.GetMongoDatabaseResponseProperties().Database),
			Mode:              toMongoDBModeDataModel(src.Properties.GetMongoDatabaseResponseProperties().Mode),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	if src.Properties.GetMongoDatabaseResponseProperties().Recipe != nil {
		converted.Properties.Recipe = toRecipeDataModel(src.Properties.GetMongoDatabaseResponseProperties().Recipe)
	}
	return converted, nil
}

// ConvertTo converts from the versioned MongoDatabase resource to version-agnostic datamodel.
func (src *MongoDatabaseResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
					Environment: to.String(src.Properties.Environment),
					Application: to.String(src.Properties.Application),
				},
				ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
				Resource:          to.String(src.Properties.Resource),
				Host:              to.String(src.Properties.Host),
				Port:              to.Int32(src.Properties.Port),
				Database:          to.String(src.Properties.Database),
				Mode:              toMongoDBModeDataModel(src.Properties.GetMongoDatabaseResponseProperties().Mode),
			},
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	if src.Properties.Secrets != nil {
		converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
			ConnectionString: to.String(src.Properties.Secrets.ConnectionString),
			Username:         to.String(src.Properties.Secrets.Username),
			Password:         to.String(src.Properties.Secrets.Password),
		}
	}
	if src.Properties.Recipe != nil {
		converted.Properties.Recipe = toRecipeDataModel(src.Properties.Recipe)
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabaseResponse resource.
func (dst *MongoDatabaseResponseResource) ConvertFrom(src conv.DataModelInterface) error {
	mongo, ok := src.(*datamodel.MongoDatabaseResponse)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(mongo.ID)
	dst.Name = to.StringPtr(mongo.Name)
	dst.Type = to.StringPtr(mongo.Type)
	dst.SystemData = fromSystemDataModel(mongo.SystemData)
	dst.Location = to.StringPtr(mongo.Location)
	dst.Tags = *to.StringMapPtr(mongo.Tags)
	dst.Properties = &MongoDatabaseResponseProperties{
		Status: &ResourceStatus{
			OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
		Environment:       to.StringPtr(mongo.Properties.Environment),
		Application:       to.StringPtr(mongo.Properties.Application),
		Mode:              fromMongoDBModeDataModel(mongo.Properties.Mode),
		Resource:          to.StringPtr(mongo.Properties.Resource),
		Host:              to.StringPtr(mongo.Properties.Host),
		Port:              to.Int32Ptr(mongo.Properties.Port),
		Database:          to.StringPtr(mongo.Properties.Database),
	}
	if mongo.Properties.Recipe.Name != "" {
		dst.Properties.GetMongoDatabaseResponseProperties().Recipe = fromRecipeDataModel(mongo.Properties.Recipe)
	}
	return nil
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
	dst.Properties = &MongoDatabaseProperties{
		Status: &ResourceStatus{
			OutputResources: rp.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
		Environment:       to.StringPtr(mongo.Properties.Environment),
		Application:       to.StringPtr(mongo.Properties.Application),
		Mode:              fromMongoDBModeDataModel(mongo.Properties.Mode),
		Resource:          to.StringPtr(mongo.Properties.Resource),
		Host:              to.StringPtr(mongo.Properties.Host),
		Port:              to.Int32Ptr(mongo.Properties.Port),
		Database:          to.StringPtr(mongo.Properties.Database),
	}
	if mongo.Properties.Recipe.Name != "" {
		dst.Properties.Recipe = fromRecipeDataModel(mongo.Properties.Recipe)
	}
	if (mongo.Properties.Secrets != datamodel.MongoDatabaseSecrets{}) {
		dst.Properties.Secrets = &MongoDatabaseSecrets{
			ConnectionString: to.StringPtr(mongo.Properties.Secrets.ConnectionString),
			Username:         to.StringPtr(mongo.Properties.Secrets.Username),
			Password:         to.StringPtr(mongo.Properties.Secrets.Password),
		}
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

func toMongoDBModeDataModel(mode *MongoDatabaseResponsePropertiesMode) datamodel.MongoDatabaseMode {
	switch *mode {
	case MongoDatabaseResponsePropertiesModeResource:
		return datamodel.MongoDatabaseModeResource
	case MongoDatabaseResponsePropertiesModeValues:
		return datamodel.MongoDatabaseModeResource
	case MongoDatabaseResponsePropertiesModeRecipe:
		return datamodel.MongoDatabaseModeRecipe
	default:
		return datamodel.MongoDatabaseModeUnknown
	}
}

func fromMongoDBModeDataModel(mode datamodel.MongoDatabaseMode) *MongoDatabaseResponsePropertiesMode {
	var convertedMode MongoDatabaseResponsePropertiesMode
	switch mode {
	case datamodel.MongoDatabaseModeResource:
		convertedMode = MongoDatabaseResponsePropertiesModeResource
	case datamodel.MongoDatabaseModeValues:
		convertedMode = MongoDatabaseResponsePropertiesModeValues
	case datamodel.MongoDatabaseModeRecipe:
		convertedMode = MongoDatabaseResponsePropertiesModeRecipe
	}
	return &convertedMode
}
