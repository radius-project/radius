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
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
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
	converted.Properties.ResourceProvisioning = toResourceProvisiongDataModel(properties.ResourceProvisioning)
	var found bool
	for _, k := range PossibleResourceProvisioningValues() {
		if ResourceProvisioning(converted.Properties.ResourceProvisioning) == k {
			found = true
			break
		}
	}
	if !found {
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
	}
	converted.Properties.Recipe = toRecipeDataModel(properties.Recipe)
	converted.Properties.Resources = toResourcesDataModel(properties.Resources)
	converted.Properties.Database = to.String(properties.Database)
	converted.Properties.Server = to.String(properties.Server)
	err := src.verifyManualInputs()
	if err != nil {
		return nil, err
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
	dst.Properties = &SQLDatabaseProperties{
		ResourceProvisioning: fromResourceProvisioningDataModel(sql.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(sql.Properties.Resources),
		Database:             to.Ptr(sql.Properties.Database),
		Server:               to.Ptr(sql.Properties.Server),
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(sql.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(sql.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(sql.Properties.Environment),
		Application:       to.Ptr(sql.Properties.Application),
	}
	if sql.Properties.ResourceProvisioning == linkrp.ResourceProvisioningRecipe {
		dst.Properties.Recipe = fromRecipeDataModel(sql.Properties.Recipe)
	}
	return nil
}

func (src *SQLDatabaseResource) verifyManualInputs() error {
	properties := src.Properties
	if properties.ResourceProvisioning != nil && *properties.ResourceProvisioning == ResourceProvisioning(linkrp.ResourceProvisioningManual) {
		if properties.Database == nil || properties.Server == nil {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("database and server are required when resourceProvisioning is %s", ResourceProvisioningManual)}
		}
	}
	return nil
}
