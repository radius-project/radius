// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const ResourceType = "Applications.Link/daprPubSubBrokers"

var NamespaceResourceType = resources.KnownType{
	Types: []resources.TypeSegment{
		{
			Type: azresources.ServiceBusNamespaces,
			Name: "*",
		},
	},
}
