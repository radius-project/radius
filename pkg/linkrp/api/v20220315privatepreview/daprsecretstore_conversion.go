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

// ConvertTo converts from the versioned DaprSecretStore resource to version-agnostic datamodel.
func (src *DaprSecretStoreResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.DaprSecretStore{
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
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
		},
	}
	v := src.Properties
	converted.Properties.ResourceProvisioning = toResourceProvisiongDataModel(v.ResourceProvisioning)
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
	converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
	converted.Properties.Type = to.String(v.Type)
	converted.Properties.Version = to.String(v.Version)
	converted.Properties.Metadata = v.Metadata
	err := converted.VerifyInputs()
	if err != nil {
		return nil, err
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprSecretStore resource.
func (dst *DaprSecretStoreResource) ConvertFrom(src v1.DataModelInterface) error {
	daprSecretStore, ok := src.(*datamodel.DaprSecretStore)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprSecretStore.ID)
	dst.Name = to.Ptr(daprSecretStore.Name)
	dst.Type = to.Ptr(daprSecretStore.Type)
	dst.SystemData = fromSystemDataModel(daprSecretStore.SystemData)
	dst.Location = to.Ptr(daprSecretStore.Location)
	dst.Tags = *to.StringMapPtr(daprSecretStore.Tags)
	dst.Properties = &DaprSecretStoreProperties{
		Recipe:               fromRecipeDataModel(daprSecretStore.Properties.Recipe),
		ResourceProvisioning: fromResourceProvisioningDataModel(daprSecretStore.Properties.ResourceProvisioning),
		ProvisioningState:    fromProvisioningStateDataModel(daprSecretStore.InternalMetadata.AsyncProvisioningState),
		Environment:          to.Ptr(daprSecretStore.Properties.Environment),
		Application:          to.Ptr(daprSecretStore.Properties.Application),
		Type:                 to.Ptr(daprSecretStore.Properties.Type),
		Version:              to.Ptr(daprSecretStore.Properties.Version),
		Metadata:             daprSecretStore.Properties.Metadata,
		ComponentName:        to.Ptr(daprSecretStore.Properties.ComponentName),
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
		},
	}
	return nil
}

func (src *DaprSecretStoreResource) verifyManualInputs() error {
	properties := src.Properties
	if properties.ResourceProvisioning != nil && *properties.ResourceProvisioning == ResourceProvisioning(linkrp.ResourceProvisioningManual) {
		if properties.Type == nil || properties.Version == nil || properties.Metadata == nil {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("type, version and metadata are required when resourceProvisioning is %s", ResourceProvisioningManual)}
		}
	}
	return nil
}
