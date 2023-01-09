// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// DaprPubSubBroker represents DaprPubSubBroker link resource.
type DaprPubSubBroker struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

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
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Topic             string               `json:"topic,omitempty"` // Topic name of the Azure ServiceBus resource. Provided by the user.
	Mode              LinkMode             `json:"mode"`
	Metadata          map[string]any       `json:"metadata,omitempty"`
	Recipe            LinkRecipe           `json:"recipe"`
	Resource          string               `json:"resource,omitempty"`
	Type              string               `json:"type,omitempty"`
	Version           string               `json:"version,omitempty"`
}
