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
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned Mongo database resource to version-agnostic datamodel and returns it,
// returning an error if any of the inputs are invalid.
func (src *MongoDatabaseResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.MongoDatabase{
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
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
		},
	}
	v := src.Properties

	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(v.ResourceProvisioning)
	if err != nil {
		return nil, err
	}

	converted.Properties.Resources = toResourcesDataModel(v.Resources)
	converted.Properties.Host = to.String(v.Host)
	converted.Properties.Port = to.Int32(v.Port)
	converted.Properties.Database = to.String(v.Database)
	converted.Properties.Username = to.String(v.Username)
	if v.Secrets != nil {
		converted.Properties.Secrets = datamodel.MongoDatabaseSecrets{
			ConnectionString: to.String(v.Secrets.ConnectionString),
			Password:         to.String(v.Secrets.Password),
		}
	}
	if converted.Properties.ResourceProvisioning != linkrp.ResourceProvisioningManual {
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
	}

	if err = converted.VerifyInputs(); err != nil {
		return nil, err
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Mongo database resource. It returns an error if the
// DataModelInterface is not a Mongo database.
func (dst *MongoDatabaseResource) ConvertFrom(src v1.DataModelInterface) error {
	mongo, ok := src.(*datamodel.MongoDatabase)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(mongo.ID)
	dst.Name = to.Ptr(mongo.Name)
	dst.Type = to.Ptr(mongo.Type)
	dst.SystemData = fromSystemDataModel(mongo.SystemData)
	dst.Location = to.Ptr(mongo.Location)
	dst.Tags = *to.StringMapPtr(mongo.Tags)

	dst.Properties = &MongoDatabaseProperties{
		Resources: fromResourcesDataModel(mongo.Properties.Resources),
		Host:      to.Ptr(mongo.Properties.Host),
		Port:      to.Ptr(mongo.Properties.Port),
		Database:  to.Ptr(mongo.Properties.Database),
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(mongo.Properties.Status.OutputResources),
		},
		ProvisioningState:    fromProvisioningStateDataModel(mongo.InternalMetadata.AsyncProvisioningState),
		Environment:          to.Ptr(mongo.Properties.Environment),
		Application:          to.Ptr(mongo.Properties.Application),
		Recipe:               fromRecipeDataModel(mongo.Properties.Recipe),
		ResourceProvisioning: fromResourceProvisioningDataModel(mongo.Properties.ResourceProvisioning),
		Username:             to.Ptr(mongo.Properties.Username),
	}

	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned MongoDatabaseSecrets instance and returns an error if
// the conversion fails.
func (dst *MongoDatabaseSecrets) ConvertFrom(src v1.DataModelInterface) error {
	mongoSecrets, ok := src.(*datamodel.MongoDatabaseSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.Ptr(mongoSecrets.ConnectionString)
	dst.Password = to.Ptr(mongoSecrets.Password)

	return nil
}

// ConvertTo converts from the versioned MongoDatabaseSecrets instance to version-agnostic datamodel.
func (src *MongoDatabaseSecrets) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.MongoDatabaseSecrets{
		ConnectionString: to.String(src.ConnectionString),
		Password:         to.String(src.Password),
	}
	return converted, nil
}
