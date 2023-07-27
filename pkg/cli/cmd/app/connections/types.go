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

package connections

// applicationGraph represnts the Radius application graph, a directed graph of interconnected
// resources that describe the application architecture and patterns of communication.
//
// The application graph supports serialization from and to JSON.
type applicationGraph struct {
	// ApplicationName is the name of the application.
	ApplicationName string `json:"applicationName"`

	// Resources is the set of resources that are part of the application. Includes environment-scoped
	// resources and cloud (non-Radius) resources that are referenced by an application-scoped
	// resource via 'connections'.
	//
	// Resources is indexed by resource ID.
	Resources map[string]resourceEntry `json:"resources"`
}

// resourceEntry represents a resource in the application graph. This could be a Radius resource like
// 'Applications.Core/containers' or a cloud resource like 'Microsoft.Storage/storageAccounts'.
type resourceEntry struct {
	node

	// Connections is the set of connections involving this resource (both in-bound and out-bound).
	Connections []connectionEntry `json:"connections"`

	// Resources is the set of output resources owned by this resource. For example an 'Applications.Core/containers'
	// running on Kubernetes may own a Kubernetes Deployment, Secret, Service, etc.
	Resources []outputResourceEntry `json:"resources"`
}

// node defines a set of shared fields for resources and output resources.
type node struct {
	// Name is the name of the resource.
	Name string `json:"name"`

	// Type is the resource type.
	Type string `json:"type"`

	// ID is the resource id.
	ID string `json:"id"`

	// Error represents a node with invalid data.
	Error string `json:"error,omitempty"`
}

// connectionEntry represents a connection between two resources in the application graph.
type connectionEntry struct {
	// Name is the name of the connection.
	Name string `json:"name"`

	// From is the source of the connection (eg: an 'Applications.Core/containers').
	From node `json:"from"`

	// To is the destination of the connection (eg: a 'Microsoft.Storage/storageAccounts').
	To node `json:"to"`
}

// outputResourceEntry represents an output resource owned by a resource in the application graph.
type outputResourceEntry struct {
	node

	// Provider is the cloud provider for the output resource. (eg: Azure)
	Provider string `json:"provider"`
}
