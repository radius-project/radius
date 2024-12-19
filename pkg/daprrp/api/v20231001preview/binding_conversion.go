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

package v20231001preview

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

// ConvertTo converts a versioned DaprBindingResource to a version-agnostic DaprBinding. It returns an error
// if the mode is not specified or if the required properties for the mode are not specified.
func (src *DaprBindingResource) ConvertTo() (v1.DataModelInterface, error) {
	daprBindingProperties := datamodel.DaprBindingProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: to.String(src.Properties.Environment),
			Application: to.String(src.Properties.Application),
		},
	}

	trackedResource := v1.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}

	internalMetadata := v1.InternalMetadata{
		UpdatedAPIVersion:      Version,
		AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
	}

	converted := &datamodel.DaprBinding{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprBindingProperties

	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(src.Properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}

	converted.Properties.Resources = toResourcesDataModel(src.Properties.Resources)
	converted.Properties.Auth = toAuthDataModel(src.Properties.Auth)

	// Note: The metadata, type, and version fields cannot be specified when using recipes since
	// the recipe is expected to create the Dapr Component manifest. However, they are required
	// when resourceProvisioning is set to manual.
	msgs := []string{}
	if converted.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if src.Properties.Recipe != nil && (!reflect.ValueOf(*src.Properties.Recipe).IsZero()) {
			msgs = append(msgs, "recipe details cannot be specified when resourceProvisioning is set to manual")
		}
		if len(src.Properties.Metadata) == 0 {
			msgs = append(msgs, "metadata must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Type == nil || *src.Properties.Type == "" {
			msgs = append(msgs, "type must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Version == nil || *src.Properties.Version == "" {
			msgs = append(msgs, "version must be specified when resourceProvisioning is set to manual")
		}
		converted.Properties.Metadata = toMetadataDataModel(src.Properties.Metadata)
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

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprBinding resource.
// If the DataModelInterface is not of the correct type, an error is returned.
func (dst *DaprBindingResource) ConvertFrom(src v1.DataModelInterface) error {
	daprBinding, ok := src.(*datamodel.DaprBinding)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprBinding.ID)
	dst.Name = to.Ptr(daprBinding.Name)
	dst.Type = to.Ptr(daprBinding.Type)
	dst.SystemData = fromSystemDataModel(daprBinding.SystemData)
	dst.Location = to.Ptr(daprBinding.Location)
	dst.Tags = *to.StringMapPtr(daprBinding.Tags)

	dst.Properties = &DaprBindingProperties{
		Environment:          to.Ptr(daprBinding.Properties.Environment),
		Application:          to.Ptr(daprBinding.Properties.Application),
		ResourceProvisioning: fromResourceProvisioningDataModel(daprBinding.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(daprBinding.Properties.Resources),
		ComponentName:        to.Ptr(daprBinding.Properties.ComponentName),
		ProvisioningState:    fromProvisioningStateDataModel(daprBinding.InternalMetadata.AsyncProvisioningState),
		Status: &ResourceStatus{
			OutputResources: toOutputResources(daprBinding.Properties.Status.OutputResources),
			Recipe:          fromRecipeStatus(daprBinding.Properties.Status.Recipe),
		},
		Auth: fromAuthDataModel(daprBinding.Properties.Auth),
	}

	if daprBinding.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		dst.Properties.Metadata = fromMetadataDataModel(daprBinding.Properties.Metadata)
		dst.Properties.Type = to.Ptr(daprBinding.Properties.Type)
		dst.Properties.Version = to.Ptr(daprBinding.Properties.Version)
	} else {
		dst.Properties.Recipe = fromRecipeDataModel(daprBinding.Properties.Recipe)
	}

	return nil
}
