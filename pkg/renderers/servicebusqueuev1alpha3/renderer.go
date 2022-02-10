// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha3

import (
	"context"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := radclient.AzureServiceBusProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	var output outputresource.OutputResource

	if to.String(properties.Resource) == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForUnmanagedResource
	}

	queueID, err := renderers.ValidateResourceID(to.String(properties.Resource), QueueResourceType, "ServiceBus Queue")
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// TODO : Need to create an output resource for service bus namespace
	output = outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureServiceBusQueue,
		ResourceKind: resourcekinds.AzureServiceBusQueue,
		Managed:      false,
		Resource: map[string]string{
			handlers.ManagedKey: "false",

			// Truncate the queue part of the ID to make an ID for the namespace
			handlers.ServiceBusNamespaceIDKey:   azresources.MakeID(queueID.SubscriptionID, queueID.ResourceGroup, queueID.Types[0]),
			handlers.ServiceBusQueueIDKey:       queueID.ID,
			handlers.ServiceBusNamespaceNameKey: queueID.Types[0].Name,
			handlers.ServiceBusQueueNameKey:     queueID.Types[1].Name,
		},
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"namespace": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceNameKey,
		},
		"queue": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusQueueNameKey,
		},
		"connectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceConnectionStringKey,
		},
		"namespaceConnectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceConnectionStringKey,
		},
		"queueConnectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusQueueConnectionStringKey,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}
