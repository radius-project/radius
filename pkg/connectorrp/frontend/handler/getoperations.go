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
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	v20220315privatepreview "github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/radrp/rest"
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
func (opctrl *GetOperations) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(opctrl.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", ProviderNamespaceName, sCtx.APIVersion), nil
}

func (opctrl *GetOperations) availableOperationsV1() *v1.PaginatedList {
	return &v1.PaginatedList{
		Value: []interface{}{
			&v1.Operation{
				Name: "Applications.Connector/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/mongoDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Get/List mongoDatabases",
					Description: "Gets/Lists mongoDatabase connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/mongoDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Create/Update mongoDatabases",
					Description: "Creates or updates a mongo database connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/mongoDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "Delete mongoDatabase",
					Description: "Deletes a mongoDatabase connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/mongoDatabases/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "mongoDatabases",
					Operation:   "List secrets",
					Description: "Lists mongoDatabase secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    ProviderNamespaceName,
					Operation:   "Register Applications.Connector resource provider",
					Description: "Registers 'Applications.Connector' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "Applications.Connector",
					Operation:   "Unregister 'Applications.Connector' resource provider",
					Description: "Unregisters 'Applications.Connector' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/sqlDatabases/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Get/List sqlDatabases",
					Description: "Gets/Lists sqlDatabase connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/sqlDatabases/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Create/Update sqlDatabases",
					Description: "Creates or updates a sql database connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/sqlDatabases/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "sqlDatabases",
					Operation:   "Delete sqlDatabase",
					Description: "Deletes a sqlDatabase connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/redisCaches/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Get/List redisCaches",
					Description: "Gets/Lists redisCache connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/redisCaches/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Create/Update redisCaches",
					Description: "Creates or updates a redisCache connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/redisCaches/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "Delete redisCache",
					Description: "Deletes a redisCache connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/redisCaches/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "redisCaches",
					Operation:   "List secrets",
					Description: "Lists redisCache secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/rabbitMQMessageQueues/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Get/List rabbitMQMessageQueues",
					Description: "Gets/Lists rabbitMQMessageQueue connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/rabbitMQMessageQueues/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Create/Update rabbitMQMessageQueues",
					Description: "Creates or updates a rabbitMQMessageQueue connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/rabbitMQMessageQueues/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "Delete rabbitMQMessageQueue",
					Description: "Deletes a rabbitMQMessageQueue connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/rabbitMQMessageQueues/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "rabbitMQMessageQueues",
					Operation:   "List secrets",
					Description: "Lists rabbitMQMessageQueue secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/extenders/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Get/List extenders",
					Description: "Gets/Lists extender connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/extenders/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Create/Update extenders",
					Description: "Creates or updates a extender connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/extenders/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "Delete extender",
					Description: "Deletes a extender connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/extenders/listsecrets/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "extenders",
					Operation:   "List secrets",
					Description: "Lists extender secrets.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprInvokeHttpRoutes/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Get/List daprInvokeHttpRoutes",
					Description: "Gets/Lists daprInvokeHttpRoutes connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprInvokeHttpRoutes/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Create/Update daprInvokeHttpRoutes",
					Description: "Creates or updates a mdaprInvokeHttpRoute connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprInvokeHttpRoutes/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprInvokeHttpRoutes",
					Operation:   "Delete daprInvokeHttpRoute",
					Description: "Deletes a daprInvokeHttpRoute connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprSecretStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Get/List daprSecretStores",
					Description: "Gets/Lists daprSecretStore connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprSecretStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Create/Update daprSecretStores",
					Description: "Creates or updates a daprSecretStore connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprSecretStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprSecretStores",
					Operation:   "Delete daprSecretStore",
					Description: "Deletes a daprSecretStore connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprStateStores/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Get/List daprStateStores",
					Description: "Gets/Lists daprStateStore connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprStateStores/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Create/Update daprStateStores",
					Description: "Creates or updates a daprStateStore connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprStateStores/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprStateStores",
					Operation:   "Delete daprStateStore",
					Description: "Deletes a daprStateStore connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprPubSubBrokers/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Get/List daprPubSubBrokers",
					Description: "Gets/Lists daprPubSubBroker connector(s).",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprPubSubBrokers/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Create/Update daprPubSubBrokers",
					Description: "Creates or updates a daprPubSubBroker connector.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Connector/daprPubSubBrokers/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "daprPubSubBrokers",
					Operation:   "Delete daprPubSubBroker",
					Description: "Deletes a daprPubSubBroker connector.",
				},
				IsDataAction: false,
			},
		},
	}
}
