/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v20220315privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned SqlDatabase resource to version-agnostic datamodel.
func (src *SQLDatabaseResource) ConvertTo() (v1.DataModelInterface, error) {
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
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetSQLDatabaseProperties().ProvisioningState),
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.GetSQLDatabaseProperties().Environment),
				Application: to.String(src.Properties.GetSQLDatabaseProperties().Application),
			},
		},
	}

	switch v := src.Properties.(type) {
	case *ResourceSQLDatabaseProperties:
		if v.Resource == nil {
			return nil, v1.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = linkrpdm.LinkModeResource
	case *ValuesSQLDatabaseProperties:
		if v.Database == nil || v.Server == nil {
			return nil, v1.NewClientErrInvalidRequest("database/server are required properties for mode 'values'")
		}
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = linkrpdm.LinkModeValues
	case *RecipeSQLDatabaseProperties:
		if v.Recipe == nil {
			return nil, v1.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Database = to.String(v.Database)
		converted.Properties.Server = to.String(v.Server)
		converted.Properties.Mode = linkrpdm.LinkModeRecipe
	default:
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetSQLDatabaseProperties().Mode))
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned SqlDatabase resource.
func (dst *SQLDatabaseResource) ConvertFrom(src v1.DataModelInterface) error {
	sql, ok := src.(*datamodel.SqlDatabase)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(sql.ID)
	dst.Name = to.Ptr(sql.Name)
	dst.Type = to.Ptr(sql.Type)
	dst.SystemData = fromSystemDataModel(sql.SystemData)
	dst.Location = to.Ptr(sql.Location)
	dst.Tags = *to.StringMapPtr(sql.Tags)
	switch sql.Properties.Mode {
	case linkrpdm.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(sql.Properties.Environment),
			Application:       to.Ptr(sql.Properties.Application),
			Resource:          to.Ptr(sql.Properties.Resource),
			Database:          to.Ptr(sql.Properties.Database),
			Server:            to.Ptr(sql.Properties.Server),
		}
	case linkrpdm.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(sql.Properties.Environment),
			Application:       to.Ptr(sql.Properties.Application),
			Database:          to.Ptr(sql.Properties.Database),
			Server:            to.Ptr(sql.Properties.Server),
		}
	case linkrpdm.LinkModeRecipe:
		mode := "recipe"
		var recipe *Recipe
		recipe = fromRecipeDataModel(sql.Properties.Recipe)
		dst.Properties = &RecipeSQLDatabaseProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
			},
			Mode:              &mode,
			ProvisioningState: fromProvisioningStateDataModel(sql.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(sql.Properties.Environment),
			Application:       to.Ptr(sql.Properties.Application),
			Recipe:            recipe,
			Database:          to.Ptr(sql.Properties.Database),
			Server:            to.Ptr(sql.Properties.Server),
		}

	}
	return nil
}
