// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/renderers"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := &radclient.RabbitMQMessageQueueResourceProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if properties.Secrets == nil {
		return renderers.RendererOutput{}, errors.New("secrets must be specified")
	}
	return renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"username": {
				Value: "",
			},
			"connectionString": {
				Value: properties.Secrets.ConnectionString,
			},
		},
	}, nil
}
