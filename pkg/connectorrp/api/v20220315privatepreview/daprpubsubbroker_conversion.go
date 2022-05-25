// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel.
func (src *DaprPubSubBrokerResource) ConvertTo() (api.DataModelInterface, error) {
	var converted *datamodel.DaprPubSubBroker
	daprPubSubproperties := datamodel.DaprPubSubBrokerProperties{
		BasicResourceProperties: basedatamodel.BasicResourceProperties{
			Status: basedatamodel.ResourceStatus{
				OutputResources: src.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources,
			},
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
		Environment:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Environment),
		Application:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Application),
		Kind:              to.String(src.Properties.GetDaprPubSubBrokerProperties().Kind),
	}
	switch v := src.Properties.(type) {
	case *DaprPubSubAzureServiceBusResourceProperties:
		converted = &datamodel.DaprPubSubBroker{
			TrackedResource: basedatamodel.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &datamodel.DaprPubSubAzureServiceBusResourceProperties{
				DaprPubSubBrokerProperties: daprPubSubproperties,
				Resource:                   to.String(v.Resource),
			},
			InternalMetadata: basedatamodel.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	case *DaprPubSubGenericResourceProperties:
		converted = &datamodel.DaprPubSubBroker{
			TrackedResource: basedatamodel.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &datamodel.DaprPubSubGenericResourceProperties{
				DaprPubSubBrokerProperties: daprPubSubproperties,
				Type:                       to.String(v.Type),
				Version:                    to.String(v.Version),
				Metadata:                   v.Metadata,
			},
			InternalMetadata: basedatamodel.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	default:
		converted = &datamodel.DaprPubSubBroker{
			TrackedResource: basedatamodel.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &daprPubSubproperties,
			InternalMetadata: basedatamodel.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	}
	return converted, nil
}

//ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
func (dst *DaprPubSubBrokerResource) ConvertFrom(src api.DataModelInterface) error {
	daprPubSub, ok := src.(*datamodel.DaprPubSubBroker)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprPubSub.ID)
	dst.Name = to.StringPtr(daprPubSub.Name)
	dst.Type = to.StringPtr(daprPubSub.Type)
	dst.SystemData = fromSystemDataModel(daprPubSub.SystemData)
	dst.Location = to.StringPtr(daprPubSub.Location)
	dst.Tags = *to.StringMapPtr(daprPubSub.Tags)
	props := &DaprPubSubBrokerProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: daprPubSub.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources,
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
		Environment:       to.StringPtr(daprPubSub.Properties.GetDaprPubSubBrokerProperties().Environment),
		Application:       to.StringPtr(daprPubSub.Properties.GetDaprPubSubBrokerProperties().Application),
	}
	switch v := daprPubSub.Properties.(type) {
	case *datamodel.DaprPubSubAzureServiceBusResourceProperties:
		dst.Properties = &DaprPubSubAzureServiceBusResourceProperties{
			DaprPubSubBrokerProperties: *props,
			Resource:                   to.StringPtr(v.Resource),
		}
	case *datamodel.DaprPubSubGenericResourceProperties:
		dst.Properties = &DaprPubSubGenericResourceProperties{
			DaprPubSubBrokerProperties: *props,
			Type:                       to.StringPtr(v.Type),
			Version:                    to.StringPtr(v.Version),
			Metadata:                   v.Metadata,
		}
	default:
		dst.Properties = props
	}

	return nil
}
