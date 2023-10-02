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

const (
	ResourceTypeName = "Applications.Core/applications"
)

var (
	ResourceTypesList = []string{
		// portableresources.MongoDatabasesResourceType,
		// portableresources.RabbitMQQueuesResourceType,
		// portableresources.RedisCachesResourceType,
		// portableresources.SqlDatabasesResourceType,
		// portableresources.DaprStateStoresResourceType,
		// portableresources.DaprSecretStoresResourceType,
		// portableresources.DaprPubSubBrokersResourceType,
		// portableresources.ExtendersResourceType,
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
		"Applications.Core/containers",
		// "Applications.Core/secretStores",
	}
)

// ApplicationGraphConnection - Describes the connection between two resources.
type ApplicationGraphConnection struct {
	// REQUIRED; The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and
	// 'Inbound' indicates indicates this connection specifies the ID of the source.
	Direction Direction `json:"direction"`

	// REQUIRED; The resource ID
	ID string `json:"id"`
}

// Direction - The direction of a connection.
type Direction string

const (
	// DirectionInbound - The resource defining this connection accepts inbound connections from the resource specified by this
	// id.
	DirectionInbound Direction = "Inbound"
	// DirectionOutbound - The resource defining this connection makes an outbound connection resource specified by this id.
	DirectionOutbound Direction = "Outbound"
)

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
