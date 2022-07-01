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
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			Resource:          to.String(src.Properties.Resource),
			Host:              to.String(src.Properties.Host),
			Port:              to.Int32(src.Properties.Port),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertTo converts from the versioned MongoDatabase resource to version-agnostic datamodel.
func (src *MongoDatabaseResource) ConvertTo() (conv.DataModelInterface, error) {
	secrets := datamodel.MongoDatabaseSecrets{}
	if src.Properties.Secrets != nil {
		secrets = datamodel.MongoDatabaseSecrets{
			ConnectionString: to.String(src.Properties.Secrets.ConnectionString),
			Username:         to.String(src.Properties.Secrets.Username),
			Password:         to.String(src.Properties.Secrets.Password),
		}
	}
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
				ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
				Environment:       to.String(src.Properties.Environment),
				Application:       to.String(src.Properties.Application),
				Resource:          to.String(src.Properties.Resource),
				Host:              to.String(src.Properties.Host),
				Port:              to.Int32(src.Properties.Port),
			},
			Secrets: secrets,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
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
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
		Environment:       to.StringPtr(mongo.Properties.Environment),
		Application:       to.StringPtr(mongo.Properties.Application),
		Resource:          to.StringPtr(mongo.Properties.Resource),
		Host:              to.StringPtr(mongo.Properties.Host),
		Port:              to.Int32Ptr(mongo.Properties.Port),
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
		MongoDatabaseResponseProperties: MongoDatabaseResponseProperties{
			BasicResourceProperties: BasicResourceProperties{
				Status: &ResourceStatus{
					OutputResources: v1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
				},
			},
			ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
			Environment:       to.StringPtr(mongo.Properties.Environment),
			Application:       to.StringPtr(mongo.Properties.Application),
			Resource:          to.StringPtr(mongo.Properties.Resource),
			Host:              to.StringPtr(mongo.Properties.Host),
			Port:              to.Int32Ptr(mongo.Properties.Port),
		},
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
