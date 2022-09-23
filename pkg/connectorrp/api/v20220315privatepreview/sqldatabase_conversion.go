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
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Resource:          to.String(src.Properties.Resource),
			Database:          to.String(src.Properties.Database),
			Server:            to.String(src.Properties.Server),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}

	if src.Properties.Recipe != nil {
		converted.Properties.Recipe.Name = to.String(src.Properties.Recipe.Name)
		if src.Properties.Recipe.Parameters != nil {
			converted.Properties.Recipe.Parameters = src.Properties.Recipe.Parameters
		}
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
		Status: &ResourceStatus{
			OutputResources: rp.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
		Environment:       to.StringPtr(sql.Properties.Environment),
		Application:       to.StringPtr(sql.Properties.Application),
		Resource:          to.StringPtr(sql.Properties.Resource),
		Database:          to.StringPtr(sql.Properties.Database),
		Server:            to.StringPtr(sql.Properties.Server),
	}

	if sql.Properties.Recipe.Name != "" {
		dst.Properties.Recipe = &Recipe{
			Name:       to.StringPtr(sql.Properties.Recipe.Name),
			Parameters: sql.Properties.Recipe.Parameters,
		}
	}

	return nil
}
