// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

// AppCoreController implements the resource types and APIs of Applications.Core resource provider.
type AppCoreController struct {
	BaseController
}

func NewAppCoreController(db db.RadrpDB, completions chan<- struct{}, scheme string) *AppCoreController {
	return &AppCoreController{
		BaseController: BaseController{
			db:          db,
			completions: completions,
			scheme:      scheme,
		},
	}
}

// GetOperations returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (ctrl *AppCoreController) GetOperations(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	ops := &armrpcv1.OperationList{
		Value: []armrpcv1.Operation{
			{
				Name: "Applications.Core/operations/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations",
				},
				IsDataAction: false,
			},
			{
				Name: "Applications.Core/environments/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "List environments",
					Description: "Get the list of environments.",
				},
				IsDataAction: false,
			},
			{
				Name: "Applications.Core/environments/write",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Create/Update environment",
					Description: "Create or update an environment.",
				},
				IsDataAction: false,
			},
			{
				Name: "Applications.Core/environments/delete",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Delete environment",
					Description: "Delete an environment.",
				},
				IsDataAction: false,
			},
			{
				Name: "Applications.Core/environments/join/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "environments",
					Operation:   "Join environment",
					Description: "Join to application environment.",
				},
				IsDataAction: false,
			},
			{
				Name: "Applications.Core/register/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Core",
					Resource:    "Applications.Core",
					Operation:   "Register Applications.Core",
					Description: "Register the subscription for Applications.Core.",
				},
				IsDataAction: false,
			},
			{
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
	return rest.NewOKResponse(ops), nil
}
