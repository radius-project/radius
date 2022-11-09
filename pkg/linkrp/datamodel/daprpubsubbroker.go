// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

type DaprPubSubBrokerKind string

const (
	DaprPubSubBrokerKindAzureServiceBus DaprPubSubBrokerKind = "pubsub.azure.servicebus"
	DaprPubSubBrokerKindGeneric         DaprPubSubBrokerKind = "generic"
	DaprPubSubBrokerKindUnknown         DaprPubSubBrokerKind = "unknown"
)

type DaprPubSubBrokerMode string

const (
	DaprPubSubBrokerModeRecipe   DaprPubSubBrokerMode = "recipe"
	DaprPubSubBrokerModeResource DaprPubSubBrokerMode = "resource"
	DaprPubSubBrokerModeValues   DaprPubSubBrokerMode = "values"
)

// DaprPubSubBroker represents DaprPubSubBroker link resource.
type DaprPubSubBroker struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (daprPubSub *DaprPubSubBroker) ResourceTypeName() string {
	return "Applications.Link/daprPubSubBrokers"
}

// DaprPubSubBrokerProperties represents the properties of DaprPubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	ProvisioningState v1.ProvisioningState   `json:"provisioningState,omitempty"`
	Kind              DaprPubSubBrokerKind   `json:"kind"`
	Topic             string                 `json:"topic,omitempty"` // Topic name of the Azure ServiceBus resource. Provided by the user.
	Mode              DaprPubSubBrokerMode   `json:"mode"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Recipe            LinkRecipe             `json:"recipe"`
	Resource          string                 `json:"resource,omitempty"`
	Type              string                 `json:"type,omitempty"`
	Version           string                 `json:"version,omitempty"`
}
