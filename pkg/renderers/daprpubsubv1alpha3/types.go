// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	ResourceType = "dapr.io.PubSubTopicComponent"
)

var TopicResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.ServiceBusNamespaces,
			Name: "*",
		},
		{
			Type: azresources.ServiceBusNamespacesTopics,
			Name: "*",
		},
	},
}

type Properties struct {
	Kind     string `json:"kind"`
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
	Topic    string `json:"topic"`
}
