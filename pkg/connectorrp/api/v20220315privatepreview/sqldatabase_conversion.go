// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned SqlDatabase resource to version-agnostic datamodel.
func (src *SQLDatabaseResource) ConvertTo() (api.DataModelInterface, error) {
	converted := &datamodel.SqlDatabase{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.SqlDatabaseProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			Resource:          to.String(src.Properties.Resource),
			Database:          to.String(src.Properties.Database),
			Server:            to.String(src.Properties.Server),
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned SqlDatabase resource.
func (dst *SQLDatabaseResource) ConvertFrom(src api.DataModelInterface) error {
	sql, ok := src.(*datamodel.SqlDatabase)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(sql.ID)
	dst.Name = to.StringPtr(sql.Name)
	dst.Type = to.StringPtr(sql.Type)
	dst.SystemData = fromSystemDataModel(sql.SystemData)
	dst.Location = to.StringPtr(sql.Location)
	dst.Tags = *to.StringMapPtr(sql.Tags)
	dst.Properties = &SQLDatabaseProperties{
		ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
		Environment:       to.StringPtr(sql.Properties.Environment),
		Application:       to.StringPtr(sql.Properties.Application),
		Resource:          to.StringPtr(sql.Properties.Resource),
		Database:          to.StringPtr(sql.Properties.Database),
		Server:            to.StringPtr(sql.Properties.Server),
	}

	return nil
}
