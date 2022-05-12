// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned MongoDatabase resource to version-agnostic datamodel.
func (src *MongoDatabaseResource) ConvertTo() (api.DataModelInterface, error) {
	secrets := datamodel.Secrets{}
	if src.Properties.Secrets != nil {
		secrets = datamodel.Secrets{
			ConnectionString: to.String(src.Properties.Secrets.ConnectionString),
			Username:         to.String(src.Properties.Secrets.Username),
			Password:         to.String(src.Properties.Secrets.Password),
		}
	}

	converted := &datamodel.MongoDatabase{
		TrackedResource: datamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.MongoDatabaseProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			Resource:          to.String(src.Properties.Resource),
			Host:              to.String(src.Properties.Host),
			Port:              int(to.Int32(src.Properties.Port)),
			Secrets:           secrets,
		},
		InternalMetadata: datamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabase resource.
func (dst *MongoDatabaseResource) ConvertFrom(src api.DataModelInterface) error {
	mongo, ok := src.(*datamodel.MongoDatabase)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(mongo.ID)
	dst.Name = to.StringPtr(mongo.Name)
	dst.Type = to.StringPtr(mongo.Type)
	dst.SystemData = fromSystemDataModel(mongo.SystemData)
	dst.Location = to.StringPtr(mongo.Location)
	dst.Tags = *to.StringMapPtr(mongo.Tags)
	dst.Properties = &MongoDatabaseProperties{
		ProvisioningState: fromProvisioningStateDataModel(mongo.Properties.ProvisioningState),
		Environment:       to.StringPtr(mongo.Properties.Environment),
		Application:       to.StringPtr(mongo.Properties.Application),
		Resource:          to.StringPtr(mongo.Properties.Resource),
		Host:              to.StringPtr(mongo.Properties.Host),
		Port:              to.Int32Ptr(int32(mongo.Properties.Port)),
	}
	if (mongo.Properties.Secrets != datamodel.Secrets{}) {
		dst.Properties.Secrets = &MongoDatabasePropertiesSecrets{
			ConnectionString: to.StringPtr(mongo.Properties.Secrets.ConnectionString),
			Username:         to.StringPtr(mongo.Properties.Secrets.Username),
			Password:         to.StringPtr(mongo.Properties.Secrets.Password),
		}
	}

	return nil
}
