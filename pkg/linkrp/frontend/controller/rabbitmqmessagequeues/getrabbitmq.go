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
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetRabbitMQMessageQueue)(nil)

// GetRabbitMQMessageQueue is the controller implementation to get the rabbitMQ conenctor resource.
type GetRabbitMQMessageQueue struct {
	ctrl.BaseController
}

// NewGetRabbitMQMessageQueue creates a new instance of GetRabbitMQMessageQueue.
func NewGetRabbitMQMessageQueue(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetRabbitMQMessageQueue{ctrl.NewBaseController(opts)}, nil
}

func (rabbitmq *GetRabbitMQMessageQueue) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.RabbitMQMessageQueue{}
	_, err := rabbitmq.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.RabbitMQMessageQueueDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
