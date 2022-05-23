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

	// switch v := src.Properties.GetDaprPubSubBrokerProperties(type) {
	// case *DaprPubSubBrokerResource:

	// case *DaprPubSubAzureServiceBusResourceProperties:
	// case *DaprPubSubGenericResourceProperties:
	// }
	converted := &datamodel.DaprPubSubBroker{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: basedatamodel.BasicResourceProperties{
				Status: basedatamodel.ResourceStatus{
					OutputResources: src.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources,
				},
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
			Environment:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Environment),
			Application:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Application),
			Resource:          to.String(src.Properties.GetDaprPubSubBrokerProperties().resource),
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
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
	dst.Properties = &DaprPubSubBrokerProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: daprPubSub.Properties.BasicResourceProperties.Status.OutputResources,
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
		Environment:       to.StringPtr(daprPubSub.Properties.Environment),
		Application:       to.StringPtr(daprPubSub.Properties.Application),
	}

	return nil
}
