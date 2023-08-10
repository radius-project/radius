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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// # Function Explanation
//
// ConvertTo converts from the versioned SqlDatabase resource to version-agnostic datamodel
// and returns an error if the inputs are invalid.
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
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
		},
	}

	properties := src.Properties

	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}
	if converted.Properties.ResourceProvisioning != linkrp.ResourceProvisioningManual {
		converted.Properties.Recipe = toRecipeDataModel(properties.Recipe)
	}
	converted.Properties.Resources = toResourcesDataModel(properties.Resources)
	converted.Properties.Database = to.String(properties.Database)
	converted.Properties.Server = to.String(properties.Server)
	converted.Properties.Port = to.Int32(properties.Port)
	converted.Properties.Username = to.String(properties.Username)
	if properties.Secrets != nil {
		converted.Properties.Secrets = datamodel.SqlDatabaseSecrets{
			ConnectionString: to.String(properties.Secrets.ConnectionString),
			Password:         to.String(properties.Secrets.Password),
		}
	}
	err = converted.VerifyInputs()
	if err != nil {
		return nil, err
	}

	return converted, nil
}

// # Function Explanation
//
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
	dst.Properties = &SQLDatabaseProperties{
		ResourceProvisioning: fromResourceProvisioningDataModel(sql.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(sql.Properties.Resources),
		Database:             to.Ptr(sql.Properties.Database),
		Server:               to.Ptr(sql.Properties.Server),
		Port:                 to.Ptr(sql.Properties.Port),
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(sql.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(sql.Properties.Environment),
		Application:       to.Ptr(sql.Properties.Application),
		Username:          to.Ptr(sql.Properties.Username),
	}
	if sql.Properties.ResourceProvisioning == linkrp.ResourceProvisioningRecipe {
		dst.Properties.Recipe = fromRecipeDataModel(sql.Properties.Recipe)
	}
	return nil
}

// # Function Explanation
//
// ConvertFrom converts from version-agnostic datamodel to the versioned SqlDatabaseSecrets instance
// and returns an error if the conversion fails.
func (dst *SQLDatabaseSecrets) ConvertFrom(src v1.DataModelInterface) error {
	sqlSecrets, ok := src.(*datamodel.SqlDatabaseSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.Ptr(sqlSecrets.ConnectionString)
	dst.Password = to.Ptr(sqlSecrets.Password)

	return nil
}

// # Function Explanation
//
// ConvertTo converts from the versioned SqlDatabaseSecrets instance to version-agnostic datamodel
// and returns an error if the conversion fails.
func (src *SQLDatabaseSecrets) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.SqlDatabaseSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Password:         to.String(src.Password),
	}
	return converted, nil
}
