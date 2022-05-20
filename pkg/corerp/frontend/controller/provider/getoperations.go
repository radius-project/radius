// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ ctrl.ControllerInterface = (*GetOperations)(nil)

// GetOperations is the controller implementation to get arm rpc available operations.
type GetOperations struct {
	ctrl.BaseController
}

// NewGetOperations creates a new GetOperations.
func NewGetOperations(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &GetOperations{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (a *GetOperations) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(a.availableOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", "Applications.Core", sCtx.APIVersion), nil
}

func (a *GetOperations) availableOperationsV1() *armrpcv1.PaginatedList {
	return &armrpcv1.PaginatedList{
		Value: []interface{}{
			&armrpcv1.Operation{
				Name: "Applications.Core/operations/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/environments/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "List environments",
					Description: "Get the list of environments.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/environments/write",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Create/Update environment",
					Description: "Create or update an environment.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/environments/delete",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Delete environment",
					Description: "Delete an environment.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/environments/join/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Join environment",
					Description: "Join to application environment.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/register/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "Applications.Core",
					Operation:   "Register Applications.Core",
					Description: "Register the subscription for Applications.Core.",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Core/unregister/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "Applications.Core",
					Operation:   "Unregister Applications.Core",
					Description: "Unregister the subscription for Applications.Core.",
				},
				IsDataAction: false,
			},
		},
	}
}
