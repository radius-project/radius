// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned SqlDatabase resource to version-agnostic datamodel.
func (src *SQLDatabaseResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.SqlDatabase{
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
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetSQLDatabaseProperties().Environment),
				Application: to.String(src.Properties.GetSQLDatabaseProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetSQLDatabaseProperties().ProvisioningState),
		},
	}

	switch v := src.Properties.(type) {
	case *ResourceSQLDatabaseProperties:
		if v.Resource == nil {
			return nil, conv.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = datamodel.LinkModeResource
	case *ValuesSQLDatabaseProperties:
		if v.Database == nil || v.Server == nil {
			return nil, conv.NewClientErrInvalidRequest("database/server are required properties for mode 'values'")
		}
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = datamodel.LinkModeValues
	case *RecipeSQLDatabaseProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = datamodel.LinkModeRecipe
	default:
		return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetSQLDatabaseProperties().Mode))
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
	switch sql.Properties.Mode {
	case datamodel.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
			Environment:       to.StringPtr(sql.Properties.Environment),
			Application:       to.StringPtr(sql.Properties.Application),
			Resource:          to.StringPtr(sql.Properties.Resource),
			Database:          to.StringPtr(sql.Properties.Database),
			Server:            to.StringPtr(sql.Properties.Server),
		}
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
			Environment:       to.StringPtr(sql.Properties.Environment),
			Application:       to.StringPtr(sql.Properties.Application),
			Database:          to.StringPtr(sql.Properties.Database),
			Server:            to.StringPtr(sql.Properties.Server),
		}
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		var recipe *Recipe
		recipe = fromRecipeDataModel(sql.Properties.Recipe)
		dst.Properties = &RecipeSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.Properties.ProvisioningState),
			Environment:       to.StringPtr(sql.Properties.Environment),
			Application:       to.StringPtr(sql.Properties.Application),
			Recipe:            recipe,
			Database:          to.StringPtr(sql.Properties.Database),
			Server:            to.StringPtr(sql.Properties.Server),
		}

	}
	return nil
}
