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
	fromResource := datamodel.FromResource{}
	if src.Properties.FromResource != nil {
		fromResource = datamodel.FromResource{
			Source: to.String(src.Properties.FromResource.Source),
		}
	}

	fromValues := datamodel.FromValues{}
	if src.Properties.FromValues != nil {
		fromValues = datamodel.FromValues{
			ConnectionString: to.String(src.Properties.FromValues.ConnectionString),
			Username:         to.String(src.Properties.FromValues.Username),
			Password:         to.String(src.Properties.FromValues.Password),
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
			Application:       to.String(src.Properties.Application),
			FromResource:      fromResource,
			FromValues:        fromValues,
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
		Application:       to.StringPtr(mongo.Properties.Application),
	}
	if (mongo.Properties.FromResource != datamodel.FromResource{}) {
		dst.Properties.FromResource = &FromResource{
			Source: to.StringPtr(mongo.Properties.FromResource.Source),
		}
	}
	if (mongo.Properties.FromValues != datamodel.FromValues{}) {
		dst.Properties.FromValues = &SecretsValues{
			ConnectionString: to.StringPtr(mongo.Properties.FromValues.ConnectionString),
			Username:         to.StringPtr(mongo.Properties.FromValues.Username),
			Password:         to.StringPtr(mongo.Properties.FromValues.Password),
		}
	}

	return nil
}
