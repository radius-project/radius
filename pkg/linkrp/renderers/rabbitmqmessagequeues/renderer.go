// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

// Render creates the output resource for the rabbitmqmessagequeues resource.
func (r Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.RabbitMQMessageQueue)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties

	_, err := renderers.ValidateApplicationID(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if properties.Secrets == (datamodel.RabbitMQSecrets{}) || properties.Secrets.ConnectionString == "" {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("secrets must be specified")
	}

	// queue name must be specified by the user
	queueName := properties.Queue
	if queueName == "" {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("queue name must be specified")
	}
	values := map[string]renderers.ComputedValueReference{
		linkrp.QueueNameKey: {
			Value: queueName,
		},
	}

	// Currently user-specfied secrets are stored in the `secrets` property of the resource, and
	// thus serialized to our database.
	//
	// TODO(#1767): We need to store these in a secret store.
	return renderers.RendererOutput{
		ComputedValues: values,
		SecretValues: map[string]rpv1.SecretValueReference{
			renderers.ConnectionStringValue: {
				Value: properties.Secrets.ConnectionString,
			},
		},
	}, nil
}
