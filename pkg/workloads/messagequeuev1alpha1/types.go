// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package messagequeuev1alpha1

import (
	"github.com/Azure/radius/pkg/azresources"
)

const Kind = "amqp.org/MessageQueue@v1alpha1"

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

// MessageQueueComponent is the definition of the service bus queue component
type MessageQueueComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   MessageQueueConfig       `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// MessageQueueConfig is the defintion of the config section
type MessageQueueConfig struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
