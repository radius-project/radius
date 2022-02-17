// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

const (
	SecretKeyRabbitMQConnectionString = "RABBITMQ_CONNECTIONSTRING"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := &radclient.RabbitMQMessageQueueResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
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
	output := renderers.RendererOutput{
		ComputedValues: values,
		SecretValues: map[string]renderers.SecretValueReference{
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}

	return output, nil
}
