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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned Extender resource to version-agnostic datamodel and returns it, or an error if the
// conversion fails.
func (src *ExtenderResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.Extender{
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
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			AdditionalProperties: src.Properties.AdditionalProperties,
			Secrets:              src.Properties.Secrets,
			ResourceRecipe:       toRecipeDataModel(src.Properties.Recipe),
		},
	}

	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(src.Properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Extender resource and returns an error if the conversion fails.
func (dst *ExtenderResource) ConvertFrom(src v1.DataModelInterface) error {
	extender, ok := src.(*datamodel.Extender)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(extender.ID)
	dst.Name = to.Ptr(extender.Name)
	dst.Type = to.Ptr(extender.Type)
	dst.SystemData = fromSystemDataModel(extender.SystemData)
	dst.Location = to.Ptr(extender.Location)
	dst.Tags = *to.StringMapPtr(extender.Tags)
	dst.Properties = &ExtenderProperties{
		Status: &ResourceStatus{
			OutputResources: toOutputResources(extender.Properties.Status.OutputResources),
		},
		ProvisioningState:    fromProvisioningStateDataModel(extender.InternalMetadata.AsyncProvisioningState),
		Environment:          to.Ptr(extender.Properties.Environment),
		Application:          to.Ptr(extender.Properties.Application),
		AdditionalProperties: extender.Properties.AdditionalProperties,
		Recipe:               fromRecipeDataModel(extender.Properties.ResourceRecipe),
		ResourceProvisioning: fromResourceProvisioningDataModel(extender.Properties.ResourceProvisioning),
		// Secrets are omitted.
	}
	return nil
}

func toResourceProvisiongDataModel(provisioning *ResourceProvisioning) (portableresources.ResourceProvisioning, error) {
	if provisioning == nil {
		return portableresources.ResourceProvisioningRecipe, nil
	}
	switch *provisioning {
	case ResourceProvisioningManual:
		return portableresources.ResourceProvisioningManual, nil
	case ResourceProvisioningRecipe:
		return portableresources.ResourceProvisioningRecipe, nil
	default:
		return "", &v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
	}
}

func fromResourceProvisioningDataModel(provisioning portableresources.ResourceProvisioning) *ResourceProvisioning {
	var converted ResourceProvisioning
	switch provisioning {
	case portableresources.ResourceProvisioningManual:
		converted = ResourceProvisioningManual
	default:
		converted = ResourceProvisioningRecipe
	}

	return &converted
}

func fromRecipeDataModel(r portableresources.LinkRecipe) *Recipe {
	return &Recipe{
		Name:       to.Ptr(r.Name),
		Parameters: r.Parameters,
	}
}

func toRecipeDataModel(r *Recipe) portableresources.LinkRecipe {
	if r == nil {
		return portableresources.LinkRecipe{
			Name: portableresources.DefaultRecipeName,
		}
	}
	recipe := portableresources.LinkRecipe{}
	if r.Name == nil {
		recipe.Name = portableresources.DefaultRecipeName
	} else {
		recipe.Name = to.String(r.Name)
	}
	if r.Parameters != nil {
		recipe.Parameters = r.Parameters
	}
	return recipe
}
