// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	v20230415preview "github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
)

var _ ctrl.Controller = (*GetOperations)(nil)

// GetOperations is the controller implementation to get arm rpc available operations.
type GetOperations struct {
	ctrl.BaseController
}

// NewGetOperations creates a new GetOperations.
func NewGetOperations(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOperations{ctrl.NewBaseController(opts)}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (opctrl *GetOperations) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20230415preview.Version:
		return rest.NewOKResponse(opctrl.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", ProviderNamespaceName, sCtx.APIVersion), nil
}

func (opctrl *GetOperations) availableOperationsV1() *v1.PaginatedList {
	return &v1.PaginatedList{
		Value: []any{
			&v1.Operation{
				Name: "Applications.Link/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/mongoDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Get/List mongoDatabases",
					Description: "Gets/Lists mongoDatabase link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/mongoDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Create/Update mongoDatabases",
					Description: "Creates or updates a mongo database link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/mongoDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Delete mongoDatabase",
					Description: "Deletes a mongoDatabase link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/mongoDatabases/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "List secrets",
					Description: "Lists mongoDatabase secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    ProviderNamespaceName,
					Operation:   "Register Applications.Link resource provider",
					Description: "Registers 'Applications.Link' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "Applications.Link",
					Operation:   "Unregister 'Applications.Link' resource provider",
					Description: "Unregisters 'Applications.Link' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/sqlDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Get/List sqlDatabases",
					Description: "Gets/Lists sqlDatabase link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/sqlDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Create/Update sqlDatabases",
					Description: "Creates or updates a sql database link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/sqlDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Delete sqlDatabase",
					Description: "Deletes a sqlDatabase link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/redisCaches/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Get/List redisCaches",
					Description: "Gets/Lists redisCache link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/redisCaches/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Create/Update redisCaches",
					Description: "Creates or updates a redisCache link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/redisCaches/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Delete redisCache",
					Description: "Deletes a redisCache link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/redisCaches/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "List secrets",
					Description: "Lists redisCache secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/rabbitMQMessageQueues/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Get/List rabbitMQMessageQueues",
					Description: "Gets/Lists rabbitMQMessageQueue link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/rabbitMQMessageQueues/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Create/Update rabbitMQMessageQueues",
					Description: "Creates or updates a rabbitMQMessageQueue link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/rabbitMQMessageQueues/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Delete rabbitMQMessageQueue",
					Description: "Deletes a rabbitMQMessageQueue link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/rabbitMQMessageQueues/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "List secrets",
					Description: "Lists rabbitMQMessageQueue secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/extenders/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Get/List extenders",
					Description: "Gets/Lists extender link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/extenders/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Create/Update extenders",
					Description: "Creates or updates a extender link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/extenders/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Delete extender",
					Description: "Deletes a extender link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/extenders/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "List secrets",
					Description: "Lists extender secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprInvokeHttpRoutes/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Get/List daprInvokeHttpRoutes",
					Description: "Gets/Lists daprInvokeHttpRoutes link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprInvokeHttpRoutes/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Create/Update daprInvokeHttpRoutes",
					Description: "Creates or updates a mdaprInvokeHttpRoute link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprInvokeHttpRoutes/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Delete daprInvokeHttpRoute",
					Description: "Deletes a daprInvokeHttpRoute link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprSecretStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Get/List daprSecretStores",
					Description: "Gets/Lists daprSecretStore link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprSecretStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Create/Update daprSecretStores",
					Description: "Creates or updates a daprSecretStore link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprSecretStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Delete daprSecretStore",
					Description: "Deletes a daprSecretStore link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprStateStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Get/List daprStateStores",
					Description: "Gets/Lists daprStateStore link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprStateStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Create/Update daprStateStores",
					Description: "Creates or updates a daprStateStore link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprStateStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Delete daprStateStore",
					Description: "Deletes a daprStateStore link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprPubSubBrokers/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Get/List daprPubSubBrokers",
					Description: "Gets/Lists daprPubSubBroker link(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprPubSubBrokers/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Create/Update daprPubSubBrokers",
					Description: "Creates or updates a daprPubSubBroker link.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Link/daprPubSubBrokers/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Delete daprPubSubBroker",
					Description: "Deletes a daprPubSubBroker link.",
				},
				IsDataAction: false,
			},
		},
	}
}
