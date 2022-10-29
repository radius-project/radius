// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel.
func (src *DaprPubSubBrokerResource) ConvertTo() (conv.DataModelInterface, error) {
	daprPubSubproperties := datamodel.DaprPubSubBrokerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: to.String(src.Properties.GetDaprPubSubBrokerProperties().Environment),
			Application: to.String(src.Properties.GetDaprPubSubBrokerProperties().Application),
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
		Kind:              toDaprPubSubBrokerKindDataModel(src.Properties.GetDaprPubSubBrokerProperties().Kind),
		Topic:             to.String(src.Properties.GetDaprPubSubBrokerProperties().Topic),
	}

	if src.Properties.GetDaprPubSubBrokerProperties().Recipe != nil {
		daprPubSubproperties.Recipe = toRecipeDataModel(src.Properties.GetDaprPubSubBrokerProperties().Recipe)
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
	converted.Properties = daprPubSubproperties
	switch v := src.Properties.(type) {
	case *DaprPubSubAzureServiceBusResourceProperties:
		converted.Properties.DaprPubSubAzureServiceBus = datamodel.DaprPubSubAzureServiceBusResourceProperties{
			Resource: to.String(v.Resource),
		}
	case *DaprPubSubGenericResourceProperties:
		converted.Properties.DaprPubSubGeneric = datamodel.DaprPubSubGenericResourceProperties{
			Type:     to.String(v.Type),
			Version:  to.String(v.Version),
			Metadata: v.Metadata,
		}
	default:
		return nil, errors.New("Kind of DaprPubSubBroker is not specified.")
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
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

	switch daprPubSub.Properties.Kind {
	case datamodel.DaprPubSubBrokerKindAzureServiceBus:
		dst.Properties = &DaprPubSubAzureServiceBusResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			Kind:              fromDaprPubSubBrokerKindDataModel(daprPubSub.Properties.Kind),
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Resource:          to.StringPtr(daprPubSub.Properties.DaprPubSubAzureServiceBus.Resource),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
		}
	case datamodel.DaprPubSubBrokerKindGeneric:
		dst.Properties = &DaprPubSubGenericResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			Kind:              fromDaprPubSubBrokerKindDataModel(daprPubSub.Properties.Kind),
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Type:              to.StringPtr(daprPubSub.Properties.DaprPubSubGeneric.Type),
			Version:           to.StringPtr(daprPubSub.Properties.DaprPubSubGeneric.Version),
			Metadata:          daprPubSub.Properties.DaprPubSubGeneric.Metadata,
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
		}
	default:
		return errors.New("Kind of DaprPubSubBroker is not specified.")
	}

	if daprPubSub.Properties.Recipe.Name != "" {
		dst.Properties.GetDaprPubSubBrokerProperties().Recipe = fromRecipeDataModel(daprPubSub.Properties.Recipe)
	}

	return nil
}

func toDaprPubSubBrokerKindDataModel(kind *DaprPubSubBrokerPropertiesKind) datamodel.DaprPubSubBrokerKind {
	switch *kind {
	case DaprPubSubBrokerPropertiesKindPubsubAzureServicebus:
		return datamodel.DaprPubSubBrokerKindAzureServiceBus
	case DaprPubSubBrokerPropertiesKindGeneric:
		return datamodel.DaprPubSubBrokerKindGeneric
	default:
		return datamodel.DaprPubSubBrokerKindUnknown
	}

}

func fromDaprPubSubBrokerKindDataModel(kind datamodel.DaprPubSubBrokerKind) *DaprPubSubBrokerPropertiesKind {
	var convertedKind DaprPubSubBrokerPropertiesKind
	switch kind {
	case datamodel.DaprPubSubBrokerKindAzureServiceBus:
		convertedKind = DaprPubSubBrokerPropertiesKindPubsubAzureServicebus
	case datamodel.DaprPubSubBrokerKindGeneric:
		convertedKind = DaprPubSubBrokerPropertiesKindGeneric
	default:
		convertedKind = DaprPubSubBrokerPropertiesKindGeneric
	}
	return &convertedKind
}
