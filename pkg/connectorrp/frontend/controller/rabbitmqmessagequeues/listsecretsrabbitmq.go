// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ base_ctrl.ControllerInterface = (*ListSecretsRabbitMQMessageQueue)(nil)

// ListSecretsRabbitMQMessageQueue is the controller implementation to list secrets for the to access the connected rabbitMQ resource resource id passed in the request body.
type ListSecretsRabbitMQMessageQueue struct {
	base_ctrl.BaseController
}

// NewListSecretsRabbitMQMessageQueue creates a new instance of ListSecretsRabbitMQMessageQueue.
func NewListSecretsRabbitMQMessageQueue(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &ListSecretsRabbitMQMessageQueue{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns secrets values for the specified RabbitMQMessageQueue resource
func (ctrl *ListSecretsRabbitMQMessageQueue) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.RabbitMQMessageQueue{}
	_, err := ctrl.GetResource(ctx, sCtx.ResourceID.String(), resource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	// TODO integrate with deploymentprocessor
	// output, err := ctrl.JobEngine.FetchSecrets(ctx, sCtx.ResourceID, resource)
	// if err != nil {
	// 	return nil, err
	// }

	versioned, _ := converter.RabbitMQSecretsDataModelToVersioned(&datamodel.RabbitMQSecrets{}, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
