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
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned Application resource to version-agnostic datamodel.
func (src *ApplicationResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
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
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
			},
		},
	}

	var extensions []datamodel.Extension
	if src.Properties.Extensions != nil {
		for _, e := range src.Properties.Extensions {
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
func (dst *ApplicationResource) ConvertFrom(src v1.DataModelInterface) error {
	app, ok := src.(*datamodel.Application)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(app.ID)
	dst.Name = to.Ptr(app.Name)
	dst.Type = to.Ptr(app.Type)
	dst.SystemData = fromSystemDataModel(app.SystemData)
	dst.Location = to.Ptr(app.Location)
	dst.Tags = *to.StringMapPtr(app.Tags)
	dst.Properties = &ApplicationProperties{
		ProvisioningState: fromProvisioningStateDataModel(app.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(app.Properties.Environment),
		Status: &ResourceStatus{
			Compute: fromEnvironmentComputeDataModel(app.Properties.Status.Compute),
		},
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
		var ann, lbl = fromExtensionClassificationFields(e)
		return &ApplicationKubernetesMetadataExtension{
			Kind:        to.Ptr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}
	case datamodel.KubernetesNamespaceExtension:
		return &ApplicationKubernetesNamespaceExtension{
			Kind:      to.Ptr(string(e.Kind)),
			Namespace: to.Ptr(e.KubernetesNamespace.Namespace),
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
	case *ApplicationKubernetesNamespaceExtension:
		if c.Namespace == nil || *c.Namespace == "" {
			return nil
		}
		return &datamodel.Extension{
			Kind: datamodel.KubernetesNamespaceExtension,
			KubernetesNamespace: &datamodel.KubeNamespaceExtension{
				Namespace: to.String(c.Namespace),
			},
		}
	}

	return nil
}
