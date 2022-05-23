// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

// DaprPubSubBroker represents DaprPubSubBroker connector resource.
type DaprPubSubBroker struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

func (redis DaprPubSubBroker) ResourceTypeName() string {
	return "Applications.Connector/daprPubSubBrokers"
}

type DaprPubSubGenericResourceProperties struct {
	DaprPubSubBrokerProperties
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Version  string                 `json:"version"`
}

type DaprPubSubAzureServiceBusResourceProperties struct {
	DaprPubSubBrokerProperties
	Resource *string `json:"resource,omitempty"`
}

// DaprPubSubBrokerProperties represents the properties of DaprPubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	basedatamodel.BasicResourceProperties
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                           `json:"environment"`
	Application       string                           `json:"application,omitempty"`
	Kind              string                           `json:"kind"`
}
