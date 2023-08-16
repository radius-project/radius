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
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrp_apiver "github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// # Function Explanation
//
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

// # Function Explanation
//
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
			OutputResources: rpv1.BuildExternalOutputResources(extender.Properties.Status.OutputResources),
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

func toResourceProvisiongDataModel(provisioning *ResourceProvisioning) (linkrp.ResourceProvisioning, error) {
	if provisioning == nil {
		return linkrp.ResourceProvisioningRecipe, nil
	}
	switch *provisioning {
	case ResourceProvisioningManual:
		return linkrp.ResourceProvisioningManual, nil
	case ResourceProvisioningRecipe:
		return linkrp.ResourceProvisioningRecipe, nil
	default:
		return "", &v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
	}
}

func fromResourceProvisioningDataModel(provisioning linkrp.ResourceProvisioning) *ResourceProvisioning {
	var converted ResourceProvisioning
	switch provisioning {
	case linkrp.ResourceProvisioningManual:
		converted = ResourceProvisioningManual
	default:
		converted = ResourceProvisioningRecipe
	}

	return &converted
}

func fromRecipeDataModel(r linkrp.LinkRecipe) *ResourceRecipe {
	return &ResourceRecipe{
		Name:       to.Ptr(r.Name),
		Parameters: r.Parameters,
	}
}

func toRecipeDataModel(r *ResourceRecipe) linkrp.LinkRecipe {
	if r == nil {
		return linkrp.LinkRecipe{
			Name: linkrp_apiver.DefaultRecipeName,
		}
	}
	recipe := linkrp.LinkRecipe{}
	if r.Name == nil {
		recipe.Name = linkrp_apiver.DefaultRecipeName
	} else {
		recipe.Name = to.String(r.Name)
	}
	if r.Parameters != nil {
		recipe.Parameters = r.Parameters
	}
	return recipe
}
