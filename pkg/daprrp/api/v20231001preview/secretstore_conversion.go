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
	"reflect"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned DaprSecretStore resource to version-agnostic datamodel and returns an error if the
// resourceProvisioning is set to manual and the required fields are not specified.
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
	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(src.Properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}

	msgs := []string{}
	if converted.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if src.Properties.Recipe != nil && (!reflect.ValueOf(*src.Properties.Recipe).IsZero()) {
			msgs = append(msgs, "recipe details cannot be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Metadata == nil || len(src.Properties.Metadata) == 0 {
			msgs = append(msgs, "metadata must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Type == nil || *src.Properties.Type == "" {
			msgs = append(msgs, "type must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Version == nil || *src.Properties.Version == "" {
			msgs = append(msgs, "version must be specified when resourceProvisioning is set to manual")
		}

		converted.Properties.Metadata = src.Properties.Metadata
		converted.Properties.Type = to.String(src.Properties.Type)
		converted.Properties.Version = to.String(src.Properties.Version)
	} else {
		if src.Properties.Metadata != nil && (!reflect.ValueOf(src.Properties.Metadata).IsZero()) {
			msgs = append(msgs, "metadata cannot be specified when resourceProvisioning is set to recipe (default)")
		}
		if src.Properties.Type != nil && (!reflect.ValueOf(*src.Properties.Type).IsZero()) {
			msgs = append(msgs, "type cannot be specified when resourceProvisioning is set to recipe (default)")
		}
		if src.Properties.Version != nil && (!reflect.ValueOf(*src.Properties.Version).IsZero()) {
			msgs = append(msgs, "version cannot be specified when resourceProvisioning is set to recipe (default)")
		}

		converted.Properties.Recipe = toRecipeDataModel(src.Properties.Recipe)
	}
	if len(msgs) > 0 {
		return nil, &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("error(s) found:\n\t%v", strings.Join(msgs, "\n\t")),
		}
	}

	return converted, nil
}

// ConvertFrom converts a version-agnostic DataModelInterface to a versionined DaprSecretStoreResource. It returns
// an error if the mode is unsupported or required properties are missing.
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
		ResourceProvisioning: fromResourceProvisioningDataModel(daprSecretStore.Properties.ResourceProvisioning),
		ProvisioningState:    fromProvisioningStateDataModel(daprSecretStore.InternalMetadata.AsyncProvisioningState),
		Environment:          to.Ptr(daprSecretStore.Properties.Environment),
		Application:          to.Ptr(daprSecretStore.Properties.Application),
		Type:                 to.Ptr(daprSecretStore.Properties.Type),
		Version:              to.Ptr(daprSecretStore.Properties.Version),
		Metadata:             daprSecretStore.Properties.Metadata,
		ComponentName:        to.Ptr(daprSecretStore.Properties.ComponentName),
		Status: &ResourceStatus{
			OutputResources: toOutputResources(daprSecretStore.Properties.Status.OutputResources),
		},
	}
	if daprSecretStore.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		dst.Properties.Metadata = daprSecretStore.Properties.Metadata
		dst.Properties.Type = to.Ptr(daprSecretStore.Properties.Type)
		dst.Properties.Version = to.Ptr(daprSecretStore.Properties.Version)
	} else {
		dst.Properties.Recipe = fromRecipeDataModel(daprSecretStore.Properties.Recipe)
	}
	return nil
}
