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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// # Function Explanation
//
// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel, validating the input
// and returning an error if any of the validation checks fail.
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

	// Note: The metadata, type, and version fields cannot be specified when using recipes since
	// the recipe is expected to create the Dapr Component manifest. However, they are required
	// when resourceProvisioning is set to manual.
	msgs := []string{}
	if converted.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
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

	if len(msgs) == 1 {
		return nil, &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: msgs[0],
		}
	} else if len(msgs) > 1 {
		return nil, &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("multiple errors were found:\n\t%v", strings.Join(msgs, "\n\t")),
		}
	}

	return converted, nil
}

// # Function Explanation
//
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
			OutputResources: rpv1.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
		},
	}

	if daprPubSub.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		dst.Properties.Metadata = daprPubSub.Properties.Metadata
		dst.Properties.Type = to.Ptr(daprPubSub.Properties.Type)
		dst.Properties.Version = to.Ptr(daprPubSub.Properties.Version)
	} else {
		dst.Properties.Recipe = fromRecipeDataModel(daprPubSub.Properties.Recipe)
	}

	return nil
}
