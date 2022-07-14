// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

type DaprPubSubBrokerKind string

const (
	DaprPubSubBrokerKindAzureServiceBus DaprPubSubBrokerKind = "pubsub.azure.servicebus"
	DaprPubSubBrokerKindGeneric         DaprPubSubBrokerKind = "generic"
	DaprPubSubBrokerKindUnknown         DaprPubSubBrokerKind = "unknown"
)

// DaprPubSubBroker represents DaprPubSubBroker connector resource.
type DaprPubSubBroker struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (daprPubSub DaprPubSubBroker) ResourceTypeName() string {
	return "Applications.Connector/daprPubSubBrokers"
}

type DaprPubSubGenericResourceProperties struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Version  string                 `json:"version,omitempty"`
}

type DaprPubSubAzureServiceBusResourceProperties struct {
	Resource string `json:"resource,omitempty"`
}

// DaprPubSubBrokerProperties represents the properties of DaprPubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	v1.BasicResourceProperties
	ProvisioningState         v1.ProvisioningState                        `json:"provisioningState,omitempty"`
	Environment               string                                      `json:"environment"`
	Application               string                                      `json:"application,omitempty"`
	Kind                      DaprPubSubBrokerKind                        `json:"kind"`
	Topic                     string                                      `json:"topic,omitempty"` // Topic name of the Azure ServiceBus resource. Set by the resource provider.
	DaprPubSubGeneric         DaprPubSubGenericResourceProperties         `json:"daprPubSubGeneric"`
	DaprPubSubAzureServiceBus DaprPubSubAzureServiceBusResourceProperties `json:"daprPubSubAzureServiceBus"`
}
