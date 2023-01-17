// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rabbitmqmessagequeues"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

var _ ctrl.Controller = (*CreateOrUpdateRabbitMQMessageQueue)(nil)

// CreateOrUpdateRabbitMQMessageQueue is the controller implementation to create or update RabbitMQMessageQueue link resource.
type CreateOrUpdateRabbitMQMessageQueue struct {
	ctrl.Operation[*datamodel.RabbitMQMessageQueue, datamodel.RabbitMQMessageQueue]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateRabbitMQMessageQueue creates a new instance of CreateOrUpdateRabbitMQMessageQueue.
func NewCreateOrUpdateRabbitMQMessageQueue(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateRabbitMQMessageQueue{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
				RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
				ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateRabbitMQMessageQueue operation.
func (rabbitmq *CreateOrUpdateRabbitMQMessageQueue) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := rabbitmq.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := rabbitmq.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := rabbitmq.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, rabbitmq.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := rabbitmq.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := rabbitmq.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	if queue, ok := deploymentOutput.ComputedValues[rabbitmqmessagequeues.QueueNameKey].(string); ok {
		newResource.Properties.Queue = queue
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = rabbitmq.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := rabbitmq.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return rabbitmq.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
