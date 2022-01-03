// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	ResourceType = "dapr.io.PubSubTopic"
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
