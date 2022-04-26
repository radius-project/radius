// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*GetConnectorOperations)(nil)

// GetConnectorOperations is the controller implementation to get arm rpc available operations.
type GetConnectorOperations struct {
	ctrl.BaseController
}

// NewGetConnectorOperations creates a new GetConnectorOperations.
func NewGetConnectorOperations(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*GetConnectorOperations, error) {
	return &GetConnectorOperations{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

// Run returns the list of available operations/permission for the resource provider at tenant level.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
func (a *GetConnectorOperations) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(a.availableConnectorOperationsV1()), nil
	}

	return rest.NewNotFoundAPIVersionResponse("operations", "Applications.Connector", sCtx.APIVersion), nil
}

func (a *GetConnectorOperations) availableConnectorOperationsV1() *armrpcv1.PaginatedList {
	return &armrpcv1.PaginatedList{
		Value: []interface{}{
			&armrpcv1.Operation{
				Name: "Applications.Connector/operations/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/read",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "mongoDatabases",
					Operation:   "Get/List mongoDatabases",
					Description: "Gets/Lists mongo database connector(s)",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/write",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "mongoDatabases",
					Operation:   "Create/Update mongoDatabases",
					Description: "Creates or updates a mongo database connector",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/mongoDatabases/delete",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "mongoDatabases",
					Operation:   "Delete mongoDatabases",
					Description: "Deletes a mongoDatabase connector",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/register/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "Applications.Connector",
					Operation:   "Register Applications.Connector resource provider",
					Description: "Registers 'Applications.Connector' resource provider with a subscription",
				},
				IsDataAction: false,
			},
			&armrpcv1.Operation{
				Name: "Applications.Connector/unregister/action",
				Display: &armrpcv1.OperationDisplayProperties{
					Provider:    "Applications.Connector",
					Resource:    "Applications.Connector",
					Operation:   "Unregister 'Applications.Connector' resource provider",
					Description: "Unregisters 'Applications.Connector' resource provider with a subscription",
				},
				IsDataAction: false,
			},
		},
	}
}
