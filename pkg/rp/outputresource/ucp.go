// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"strings"

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func FromUCPID(id resources.ID) OutputResource {
	// Blank resource id => blank output resource
	if len(id.ScopeSegments()) == 0 {
		return OutputResource{}
	}

	firstScope := id.ScopeSegments()[0].Type
	if strings.EqualFold(resources.SubscriptionsSegment, firstScope) {
		// If this starts with a subscription ID then it's an Azure resource
		return OutputResource{
			Identity: resourcemodel.NewARMIdentity(&resourcemodel.ResourceType{Type: id.Type(), Provider: resourcemodel.ProviderAzure}, id.String(), "ignore"),
		}
	}

	if strings.EqualFold("azure", firstScope) {
		return OutputResource{
			Identity: resourcemodel.NewARMIdentity(&resourcemodel.ResourceType{Type: id.Type(), Provider: resourcemodel.ProviderAzure}, id.String(), "ignore"),
		}
	}

	if strings.EqualFold("aws", firstScope) {
		return OutputResource{
			Identity: resourcemodel.NewUCPIdentity(&resourcemodel.ResourceType{Type: id.Type(), Provider: resourcemodel.ProviderAWS}, id.String()),
		}
	}

	if strings.EqualFold("kubernetes", firstScope) {
		group, kind, _ := strings.Cut(id.Type(), "/")
		if strings.EqualFold(group, "core") {
			group = ""
		}

		return OutputResource{
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     id.Type(),
					Provider: resourcemodel.ProviderKubernetes,
				},
				Data: resourcemodel.KubernetesIdentity{
					Kind:       kind,
					APIVersion: group + "/v1", // TODO flow API Version or get rid of it....
					Namespace:  id.FindScope("namespaces"),
					Name:       id.Name(),
				},
			},
		}
	}

	return OutputResource{}
}
