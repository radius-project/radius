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

// ConvertTo converts a versioned DaprPubSubBrokerResource to a version-agnostic DaprPubSubBroker. It returns an error
// if the mode is not specified or if the required properties for the mode are not specified.
func (src *DaprPubSubBrokerResource) ConvertTo() (v1.DataModelInterface, error) {
	daprPubSubproperties := datamodel.DaprPubSubBrokerProperties{
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
	converted := &datamodel.DaprPubSubBroker{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprPubSubproperties

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

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
// If the DataModelInterface is not of the correct type, an error is returned.
func (dst *DaprPubSubBrokerResource) ConvertFrom(src v1.DataModelInterface) error {
	daprPubSub, ok := src.(*datamodel.DaprPubSubBroker)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprPubSub.ID)
	dst.Name = to.Ptr(daprPubSub.Name)
	dst.Type = to.Ptr(daprPubSub.Type)
	dst.SystemData = fromSystemDataModel(daprPubSub.SystemData)
	dst.Location = to.Ptr(daprPubSub.Location)
	dst.Tags = *to.StringMapPtr(daprPubSub.Tags)

	dst.Properties = &DaprPubSubBrokerProperties{
		Environment:          to.Ptr(daprPubSub.Properties.Environment),
		Application:          to.Ptr(daprPubSub.Properties.Application),
		ResourceProvisioning: fromResourceProvisioningDataModel(daprPubSub.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(daprPubSub.Properties.Resources),
		ComponentName:        to.Ptr(daprPubSub.Properties.ComponentName),
		ProvisioningState:    fromProvisioningStateDataModel(daprPubSub.InternalMetadata.AsyncProvisioningState),
		Status: &ResourceStatus{
			OutputResources: toOutputResources(daprPubSub.Properties.Status.OutputResources),
			Recipe:          fromRecipeStatus(daprPubSub.Properties.Status.Recipe),
		},
		Auth: fromAuthDataModel(daprPubSub.Properties.Auth),
	}

	if daprPubSub.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		dst.Properties.Metadata = fromMetadataDataModel(daprPubSub.Properties.Metadata)
		dst.Properties.Type = to.Ptr(daprPubSub.Properties.Type)
		dst.Properties.Version = to.Ptr(daprPubSub.Properties.Version)
	} else {
		dst.Properties.Recipe = fromRecipeDataModel(daprPubSub.Properties.Recipe)
	}

	return nil
}
