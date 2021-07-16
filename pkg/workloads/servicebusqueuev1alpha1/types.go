// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/radrp/resources"
)

const Kind = "azure.com/ServiceBusQueue@v1alpha1"

var QueueResourceType = resources.KnownType{
	Types: []resources.ResourceType{
		{
			Type: azresources.ServiceBusNamespaces,
			Name: "*",
		},
		{
			Type: azresources.ServiceBusNamespacesQueues,
			Name: "*",
		},
	},
}

// ServiceBusQueueComponent is the definition of the service bus queue component
type ServiceBusQueueComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    ServiceBusQueueConfig    `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// ServiceBusQueueConfig is the defintion of the config section
type ServiceBusQueueConfig struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
