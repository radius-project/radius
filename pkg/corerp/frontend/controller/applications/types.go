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

package applications

import (
	cntr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	ext_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/extenders"
	gtwy_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	hrt_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/httproutes"
	sstr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
)

const (
	ResourceTypeName = "Applications.Core/applications"
)

var (
	ResourceTypesList = []string{
		ds_ctrl.MongoDatabasesResourceType,
		msg_ctrl.RabbitMQQueuesResourceType,
		ds_ctrl.RedisCachesResourceType,
		ds_ctrl.SqlDatabasesResourceType,
		dapr_ctrl.DaprStateStoresResourceType,
		dapr_ctrl.DaprSecretStoresResourceType,
		dapr_ctrl.DaprPubSubBrokersResourceType,
		ext_ctrl.ResourceTypeName,
		gtwy_ctrl.ResourceTypeName,
		hrt_ctrl.ResourceTypeName,
		cntr_ctrl.ResourceTypeName,
		sstr_ctrl.ResourceTypeName,
	}
)

/*
// ApplicationGraphConnection - Describes the connection between two resources.
type ApplicationGraphConnection struct {
	// REQUIRED; The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and
	// 'Inbound' indicates indicates this connection specifies the ID of the source.
	Direction corerpv20231001preview.Direction `json:"direction"`

	// REQUIRED; The resource ID
	ID string `json:"id"`
}

// ApplicationGraphOutputResource - Describes an output resource that comprises an application graph resource.
type ApplicationGraphOutputResource struct {
	// REQUIRED; The resource ID.
	ID string `json:"id"`

	// REQUIRED; The resource name.
	Name string `json:"name"`

	// REQUIRED; The resource type.
	Type string `json:"type"`

	Error string `json:"error,omitempty"`
}

// ApplicationGraphResource - Describes a resource in the application graph.
type ApplicationGraphResource struct {
	Connections []ApplicationGraphConnection `json:"connections,omitempty"`

	ID string `json:"id"`

	// REQUIRED; The resource name.
	Name string `json:"name"`

	// REQUIRED; The resources that comprise this resource.
	Resources []ApplicationGraphOutputResource `json:"outputResources"`

	// REQUIRED; The resource type.
	Type string `json:"type"`

	ProvisioningState string `json:"provisioningState"`
}

// ApplicationGraphResponse - Describes the application architecture and its dependencies.
type ApplicationGraphResponse struct {
	// REQUIRED; The resources in the application graph.
	Resources []*ApplicationGraphResource `json:"resources"`
}

type ConnectionProperties struct {
	// REQUIRED; The source of the connection
	Source *string
}

type ContainerPortProperties struct {
	// REQUIRED; The listening port number
	ContainerPort *int32

	// Specifies the port that will be exposed by this container. Must be set when value different from containerPort is desired
	Port *int32

	// Protocol in use by the port
	Protocol *PortProtocol

	// Specifies a route provided by this port
	Provides *string

	// Specifies the URL scheme of the communication protocol. Consumers can use the scheme to construct a URL. The value defaults
	// to 'http' or 'https' depending on the port value
	Scheme *string
}

// PortProtocol - The protocol in use by the port
type PortProtocol string

const (
	// PortProtocolTCP - TCP protocol
	PortProtocolTCP PortProtocol = "TCP"
	// PortProtocolUDP - UDP protocol
	PortProtocolUDP PortProtocol = "UDP"
)

*/
