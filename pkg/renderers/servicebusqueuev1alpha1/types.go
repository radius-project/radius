// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
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

type Properties struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
