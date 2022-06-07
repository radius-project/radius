// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	v20220315privatepreview "github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetOperations)(nil)

// GetOperations is the controller implementation to get arm rpc available operations.
type GetOperations struct {
	ctrl.BaseController
}

// NewGetOperations creates a new GetOperations.
func NewGetOperations(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &GetOperations{ctrl.NewBaseController(ds, sm)}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (opctrl *GetOperations) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(opctrl.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", "Applications.Core", sCtx.APIVersion), nil
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
		},
	}
}
