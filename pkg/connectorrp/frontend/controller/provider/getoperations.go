// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ controller.ControllerInterface = (*GetOperations)(nil)

// GetOperations is the controller implementation to get available operations for Applications.Connector.
type GetOperations struct {
	controller.BaseController
}

// NewGetOperations creates a new instance of GetOperations.
func NewGetOperations(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (controller.ControllerInterface, error) {
	return &GetOperations{
		BaseController: controller.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (operationsCtrl *GetOperations) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(operationsCtrl.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", Namespace, sCtx.APIVersion), nil
}

func (a *GetOperations) availableOperationsV1() *armrpcv1.PaginatedList {
	return &armrpcv1.PaginatedList{
		Value: []interface{}{
			&armrpcv1.Operation{
				Name: "Applications.Connector/operations/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "mongoDatabases",
					Operation:   "Get/List mongoDatabases",
					Description: "Gets/Lists mongoDatabase connector(s).",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/write",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "mongoDatabases",
					Operation:   "Create/Update mongoDatabases",
					Description: "Creates or updates a mongo database connector.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/delete",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "mongoDatabases",
					Operation:   "Delete mongoDatabase",
					Description: "Deletes a mongoDatabase connector.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/register/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "Applications.Connector",
					Operation:   "Register Applications.Connector resource provider",
					Description: "Registers 'Applications.Connector' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/unregister/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    Namespace,
					Resource:    "Applications.Connector",
					Operation:   "Unregister 'Applications.Connector' resource provider",
					Description: "Unregisters 'Applications.Connector' resource provider with a subscription.",
				},
				IsDataAction: false,
			},
		},
	}
}
