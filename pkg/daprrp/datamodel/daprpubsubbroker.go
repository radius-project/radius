// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprPubSubBroker represents DaprPubSubBroker link resource.
type DaprPubSubBroker struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrpdm.LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprPubSubBroker) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprPubSubBroker) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprPubSubBroker) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (daprPubSub *DaprPubSubBroker) ResourceTypeName() string {
	return linkrp.N_DaprPubSubBrokersResourceType
}

// DaprPubSubBrokerProperties represents the properties of DaprPubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Topic    string            `json:"topic,omitempty"` // Topic name of the Azure ServiceBus resource. Provided by the user.
	Mode     linkrpdm.LinkMode `json:"mode"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Recipe   linkrp.LinkRecipe `json:"recipe"`
	Resource string            `json:"resource,omitempty"`
	Type     string            `json:"type,omitempty"`
	Version  string            `json:"version,omitempty"`
}
