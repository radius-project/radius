// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/renderers"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.RabbitMQMessageQueueResourceProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if properties.Secrets == nil {
		return renderers.RendererOutput{}, errors.New("secrets must be specified")
	}

	// queue name must be specified by the user
	queueName := to.String(properties.Queue)
	if queueName == "" {
		return renderers.RendererOutput{}, fmt.Errorf("queue name must be specified")
	}
	values := map[string]renderers.ComputedValueReference{
		"queue": {
			Value: queueName,
		},
	}

	// Currently user-specfied secrets are stored in the `secrets` property of the resource, and
	// thus serialized to our database.
	//
	// TODO(#1767): We need to store these in a secret store.
	return renderers.RendererOutput{
		ComputedValues: values,
		SecretValues: map[string]renderers.SecretValueReference{
			"connectionString": {
				Value: *properties.Secrets.ConnectionString,
			},
		},
	}, nil
}
