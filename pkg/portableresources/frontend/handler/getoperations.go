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

package handler

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	v20220315privatepreview "github.com/project-radius/radius/pkg/portableresources/api/v20220315privatepreview"
)

var _ ctrl.Controller = (*GetOperations)(nil)

// GetOperations is the controller implementation to get arm rpc available operations.
type GetOperations struct {
	ctrl.BaseController
}

// NewGetOperations creates a new GetOperations controller and returns it, or returns an error if one occurs.
func NewGetOperations(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOperations{ctrl.NewBaseController(opts)}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (opctrl *GetOperations) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(opctrl.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", PortableResourcesNamespace, sCtx.APIVersion), nil
}

func (opctrl *GetOperations) availableOperationsV1() *v1.PaginatedList {
	return &v1.PaginatedList{
		Value: []any{
			&v1.Operation{
				Name: "Applications.Dapr/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/mongoDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "mongoDatabases",
					Operation:   "Get/List mongoDatabases",
					Description: "Gets/Lists mongoDatabase resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/mongoDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "mongoDatabases",
					Operation:   "Create/Update mongoDatabases",
					Description: "Creates or updates a mongo database resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/mongoDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "mongoDatabases",
					Operation:   "Delete mongoDatabase",
					Description: "Deletes a mongoDatabase resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/mongoDatabases/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "mongoDatabases",
					Operation:   "List secrets",
					Description: "Lists mongoDatabase secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    DatastoresProviderNamespace,
					Operation:   "Register Applications.Datastores resource provider",
					Description: "Registers 'Applications.Datastores' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    DaprProviderNamespace,
					Operation:   "Register Applications.Dapr resource provider",
					Description: "Registers 'Applications.Dapr' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    MessagingProviderNamespace,
					Operation:   "Register Applications.Messaging resource provider",
					Description: "Registers 'Applications.Messaging' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "Applications.Datastores",
					Operation:   "Unregister 'Applications.Datastores' resource provider",
					Description: "Unregisters 'Applications.Datastores' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "Applications.Datastores",
					Operation:   "Unregister 'Applications.Dapr' resource provider",
					Description: "Unregisters 'Applications.Dapr' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "Applications.Datastores",
					Operation:   "Unregister 'Applications.Messaging' resource provider",
					Description: "Unregisters 'Applications.Messaging' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/sqlDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "sqlDatabases",
					Operation:   "Get/List sqlDatabases",
					Description: "Gets/Lists sqlDatabase resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/sqlDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "sqlDatabases",
					Operation:   "Create/Update sqlDatabases",
					Description: "Creates or updates a sql database resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/sqlDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "sqlDatabases",
					Operation:   "Delete sqlDatabase",
					Description: "Deletes a sqlDatabase resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/redisCaches/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "redisCaches",
					Operation:   "Get/List redisCaches",
					Description: "Gets/Lists redisCache resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/redisCaches/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "redisCaches",
					Operation:   "Create/Update redisCaches",
					Description: "Creates or updates a redisCache resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/redisCaches/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "redisCaches",
					Operation:   "Delete redisCache",
					Description: "Deletes a redisCache resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Datastores/redisCaches/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    DatastoresProviderNamespace,
					Resource:    "redisCaches",
					Operation:   "List secrets",
					Description: "Lists redisCache secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/rabbitMQQueues/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "rabbitMQQueues",
					Operation:   "Get/List rabbitMQQueues",
					Description: "Gets/Lists rabbitMQQueue resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/rabbitMQQueues/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "rabbitMQQueues",
					Operation:   "Create/Update rabbitMQQueues",
					Description: "Creates or updates a rabbitMQQueue resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/rabbitMQQueues/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "rabbitMQQueues",
					Operation:   "Delete rabbitMQQueue",
					Description: "Deletes a rabbitMQQueue resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Messaging/rabbitMQQueues/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    MessagingProviderNamespace,
					Resource:    "rabbitMQQueues",
					Operation:   "List secrets",
					Description: "Lists rabbitMQQueue secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/secretStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprSecretStores",
					Operation:   "Get/List daprSecretStores",
					Description: "Gets/Lists daprSecretStore resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/secretStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprSecretStores",
					Operation:   "Create/Update daprSecretStores",
					Description: "Creates or updates a daprSecretStore resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/secretStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprSecretStores",
					Operation:   "Delete daprSecretStore",
					Description: "Deletes a daprSecretStore resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/stateStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprStateStores",
					Operation:   "Get/List daprStateStores",
					Description: "Gets/Lists daprStateStore resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/stateStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprStateStores",
					Operation:   "Create/Update daprStateStores",
					Description: "Creates or updates a daprStateStore resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/stateStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprStateStores",
					Operation:   "Delete daprStateStore",
					Description: "Deletes a daprStateStore resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/pubSubBrokers/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprPubSubBrokers",
					Operation:   "Get/List daprPubSubBrokers",
					Description: "Gets/Lists daprPubSubBroker resource(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/pubSubBrokers/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprPubSubBrokers",
					Operation:   "Create/Update daprPubSubBrokers",
					Description: "Creates or updates a daprPubSubBroker resource.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Dapr/pubSubBrokers/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    DaprProviderNamespace,
					Resource:    "daprPubSubBrokers",
					Operation:   "Delete daprPubSubBroker",
					Description: "Deletes a daprPubSubBroker resource.",
				},
				IsDataAction: false,
			},
		},
	}
}
