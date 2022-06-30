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

// ConvertTo converts from the versioned SqlDatabase resource to version-agnostic datamodel.
func (src *SQLDatabaseResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.SqlDatabase{
		TrackedResource: v1.TrackedResource{
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
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned SqlDatabase resource.
func (dst *SQLDatabaseResource) ConvertFrom(src conv.DataModelInterface) error {
	sql, ok := src.(*datamodel.SqlDatabase)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(sql.ID)
	dst.Name = to.StringPtr(sql.Name)
	dst.Type = to.StringPtr(sql.Type)
	dst.SystemData = fromSystemDataModel(sql.SystemData)
	dst.Location = to.StringPtr(sql.Location)
	dst.Tags = *to.StringMapPtr(sql.Tags)
	dst.Properties = &SQLDatabaseProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
		Environment:       to.StringPtr(sql.Properties.Environment),
		Application:       to.StringPtr(sql.Properties.Application),
		Resource:          to.StringPtr(sql.Properties.Resource),
		Database:          to.StringPtr(sql.Properties.Database),
		Server:            to.StringPtr(sql.Properties.Server),
	}

	return nil
}
