// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"
	"errors"
	"net/http"

	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetRabbitMQMessageQueue)(nil)

// GetRabbitMQMessageQueue is the controller implementation to get the rabbitMQ conenctor resource.
type GetRabbitMQMessageQueue struct {
	ctrl.BaseController
}

// NewGetRabbitMQMessageQueue creates a new instance of GetRabbitMQMessageQueue.
func NewGetRabbitMQMessageQueue(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &GetRabbitMQMessageQueue{ctrl.NewBaseController(ds, sm)}, nil
}

func (rabbitmq *GetRabbitMQMessageQueue) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

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
