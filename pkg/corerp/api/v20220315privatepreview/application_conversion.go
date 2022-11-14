// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Application resource to version-agnostic datamodel.
func (src *ApplicationResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.Application{
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
		Properties: datamodel.ApplicationProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
			},
		},
	}

	var extensions []datamodel.Extension
	if src.Properties.Extensions != nil {
		for _, e := range src.Properties.Extensions {
			extensions = append(extensions, toAppExtensionDataModel(e))
		}
		converted.Properties.Extensions = extensions
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Application resource.
func (dst *ApplicationResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	app, ok := src.(*datamodel.Application)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(app.ID)
	dst.Name = to.StringPtr(app.Name)
	dst.Type = to.StringPtr(app.Type)
	dst.SystemData = fromSystemDataModel(app.SystemData)
	dst.Location = to.StringPtr(app.Location)
	dst.Tags = *to.StringMapPtr(app.Tags)
	dst.Properties = &ApplicationProperties{
		ProvisioningState: fromProvisioningStateDataModel(app.InternalMetadata.AsyncProvisioningState),
		Environment:       to.StringPtr(app.Properties.Environment),
	}

	var extensions []ApplicationExtensionClassification
	if app.Properties.Extensions != nil {
		for _, e := range app.Properties.Extensions {
			extensions = append(extensions, fromAppExtensionClassificationDataModel(e))
		}
		dst.Properties.Extensions = extensions
	}

	return nil
}

// fromAppExtensionClassificationDataModel: Converts from base datamodel to versioned datamodel
func fromAppExtensionClassificationDataModel(e datamodel.Extension) ApplicationExtensionClassification {
	switch e.Kind {
	case datamodel.KubernetesMetadata:
		var ann, lbl = getFromExtensionClassificationFields(e)
		converted := ApplicationKubernetesMetadataExtension{
			Kind:        to.StringPtr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}

		return converted.GetApplicationExtension()
	}

	return nil
}

// toAppExtensionDataModel: Converts from versioned datamodel to base datamodel
func toAppExtensionDataModel(e ApplicationExtensionClassification) datamodel.Extension {
	switch c := e.(type) {
	case *ApplicationKubernetesMetadataExtension:

		converted := datamodel.Extension{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.BaseKubernetesMetadataExtension{
				Annotations: to.StringMap(c.Annotations),
				Labels:      to.StringMap(c.Labels),
			},
		}
		return converted
	}

	return datamodel.Extension{}
}
