// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

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
		for i, e := range src.Properties.Extensions {
			if kube, ok := e.(*ApplicationKubernetesNamespaceExtension); ok {
				if converted.Properties.KubernetesOptions != nil {
					return nil, &conv.ErrModelConversion{
						PropertyName: fmt.Sprintf("$.properties.extensions[%d]", i),
						ValidValue:   "duplicated kuberentesNamespace extension",
					}
				}

				converted.Properties.KubernetesOptions = &datamodel.KubernetesComputeProperties{Namespace: to.String(kube.Namespace)}
				continue
			}

			//TODO : Check whether namespace extension has already been defined
			ext := toAppExtensionDataModel(e)
			if ext != nil {
				extensions = append(extensions, *ext)
			}
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

	if app.Properties.KubernetesOptions != nil {
		if dst.Properties.Extensions == nil {
			dst.Properties.Extensions = []ApplicationExtensionClassification{}
		}

		dst.Properties.Extensions = append(dst.Properties.Extensions, &ApplicationKubernetesNamespaceExtension{
			Kind:      to.StringPtr("kubernetesNamespace"),
			Namespace: to.StringPtr(app.Properties.KubernetesOptions.Namespace),
		})
	}

	return nil
}

// fromAppExtensionClassificationDataModel: Converts from base datamodel to versioned datamodel
func fromAppExtensionClassificationDataModel(e datamodel.Extension) ApplicationExtensionClassification {
	switch e.Kind {
	case datamodel.KubernetesMetadata:
		var ann, lbl = getFromExtensionClassificationFields(e)
		return &ApplicationKubernetesMetadataExtension{
			Kind:        to.StringPtr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}
	}

	return nil
}

// toAppExtensionDataModel: Converts from versioned datamodel to base datamodel
func toAppExtensionDataModel(e ApplicationExtensionClassification) *datamodel.Extension {
	switch c := e.(type) {
	case *ApplicationKubernetesMetadataExtension:
		return &datamodel.Extension{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: to.StringMap(c.Annotations),
				Labels:      to.StringMap(c.Labels),
			},
		}
	}

	return nil
}
