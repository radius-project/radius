// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	Kind         = "azure.com/ServiceBusQueue@v1alpha1"
	ResourceType = "azure.com.ServiceBusQueueComponent"
)

var QueueResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
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
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   ServiceBusQueueConfig    `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// ServiceBusQueueConfig is the defintion of the config section
type ServiceBusQueueConfig struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
