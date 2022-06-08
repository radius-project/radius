// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"errors"
	"reflect"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel.
func (src *DaprPubSubBrokerResource) ConvertTo() (conv.DataModelInterface, error) {
	outputResources := v1.ResourceStatus{}.OutputResources
	if src.Properties.GetDaprPubSubBrokerProperties().Status != nil {
		outputResources = src.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources
	}
	daprPubSubproperties := datamodel.DaprPubSubBrokerProperties{
		BasicResourceProperties: v1.BasicResourceProperties{
			Status: v1.ResourceStatus{
				OutputResources: outputResources,
			},
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
		Environment:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Environment),
		Application:       to.String(src.Properties.GetDaprPubSubBrokerProperties().Application),
		Kind:              to.String(src.Properties.GetDaprPubSubBrokerProperties().Kind),
	}
	trackedResource := v1.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}
	internalMetadata := v1.InternalMetadata{
		UpdatedAPIVersion: Version,
	}
	converted := &datamodel.DaprPubSubBroker{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	switch v := src.Properties.(type) {
	case *DaprPubSubAzureServiceBusResourceProperties:
		converted.Properties = &datamodel.DaprPubSubAzureServiceBusResourceProperties{
			DaprPubSubBrokerProperties: daprPubSubproperties,
			Resource:                   to.String(v.Resource),
		}
	case *DaprPubSubGenericResourceProperties:
		converted.Properties = &datamodel.DaprPubSubGenericResourceProperties{
			DaprPubSubBrokerProperties: daprPubSubproperties,
			Type:                       to.String(v.Type),
			Version:                    to.String(v.Version),
			Metadata:                   v.Metadata,
		}
	default:
		return nil, errors.New("Kind of DaprPubSubBroker is not specified.")
	}
	return converted, nil
}

//ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
func (dst *DaprPubSubBrokerResource) ConvertFrom(src conv.DataModelInterface) error {
	daprPubSub, ok := src.(*datamodel.DaprPubSubBroker)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprPubSub.ID)
	dst.Name = to.StringPtr(daprPubSub.Name)
	dst.Type = to.StringPtr(daprPubSub.Type)
	dst.SystemData = fromSystemDataModel(daprPubSub.SystemData)
	dst.Location = to.StringPtr(daprPubSub.Location)
	dst.Tags = *to.StringMapPtr(daprPubSub.Tags)
	var outputresources []map[string]interface{}
	if !(reflect.DeepEqual(daprPubSub.Properties.GetDaprPubSubBrokerProperties().Status, v1.ResourceStatus{})) {
		outputresources = daprPubSub.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources
	}
	props := &DaprPubSubBrokerProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: outputresources,
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
		return errors.New("Kind of DaprPubSubBroker is not specified.")
	}

	return nil
}
