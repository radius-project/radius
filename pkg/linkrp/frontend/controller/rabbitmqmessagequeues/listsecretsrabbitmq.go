/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rabbitmqmessagequeues

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

var _ ctrl.Controller = (*ListSecretsRabbitMQMessageQueue)(nil)

// ListSecretsRabbitMQMessageQueue is the controller implementation to list secrets for the to access the connected rabbitMQ resource resource id passed in the request body.
// (TODO: Remove this function once RabbitMQMessageQueue is removed from the API)
type ListSecretsRabbitMQMessageQueue struct {
	ctrl.Operation[*datamodel.RabbitMQMessageQueue, datamodel.RabbitMQMessageQueue]
}

// NewListSecretsRabbitMQMessageQueue creates a new instance of ListSecretsRabbitMQMessageQueue.
// (TODO: Remove this function once RabbitMQMessageQueue is removed)
func NewListSecretsRabbitMQMessageQueue(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsRabbitMQMessageQueue{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
				RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
				ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
			}),
	}, nil
}

// Run returns secrets values for the specified RabbitMQMessageQueue resource
// (TODO: Remove this function once RabbitMQMessageQueue is removed)
func (ctrl *ListSecretsRabbitMQMessageQueue) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Link/rabbitMQMessageQueues/<resource_name>/listsecrets
	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	redisSecrets := datamodel.RabbitMQSecrets{}

	if connectionString, ok := resource.SecretValues[renderers.ConnectionStringValue]; ok {
		redisSecrets.ConnectionString = connectionString.Value
	}

	versioned, _ := converter.RabbitMQSecretsDataModelToVersioned(&redisSecrets, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
