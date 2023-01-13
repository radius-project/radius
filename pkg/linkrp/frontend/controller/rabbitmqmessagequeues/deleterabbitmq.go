// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*DeleteRabbitMQMessageQueue)(nil)

// DeleteRabbitMQMessageQueue is the controller implementation to delete rabbitmq link resource.
type DeleteRabbitMQMessageQueue struct {
	ctrl.Operation[*datamodel.RabbitMQMessageQueue, datamodel.RabbitMQMessageQueue]

	KubeClient runtimeclient.Client
	dp         deployment.DeploymentProcessor
}

// NewDeleteRabbitMQMessageQueue creates a new instance DeleteRabbitMQMessageQueue.
func NewDeleteRabbitMQMessageQueue(opts fctrl.Options) (ctrl.Controller, error) {
	return &DeleteRabbitMQMessageQueue{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
				RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
				ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

func (rmq *DeleteRabbitMQMessageQueue) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := rmq.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := rmq.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	err = rmq.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	if err := rmq.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
