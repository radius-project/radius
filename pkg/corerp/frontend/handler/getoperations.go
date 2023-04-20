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
	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
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
				Name: "Applications.Core/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/environments/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "environments",
					Operation:   "List environments",
					Description: "Get the list of environments.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/environments/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "environments",
					Operation:   "Create/Update environment",
					Description: "Create or update an environment.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/environments/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "environments",
					Operation:   "Delete environment",
					Description: "Delete an environment.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/environments/getmetadata/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "environments",
					Operation:   "Get recipe metadata",
					Description: "Get recipe metadata.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/environments/join/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "environments",
					Operation:   "Join environment",
					Description: "Join to application environment.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/register/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    ProviderNamespaceName,
					Operation:   "Register Applications.Core",
					Description: "Register the subscription for Applications.Core.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/unregister/action",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    ProviderNamespaceName,
					Operation:   "Unregister Applications.Core",
					Description: "Unregister the subscription for Applications.Core.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/httproutes/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "httproutes",
					Operation:   "List httproutes",
					Description: "Get the list of httproutes.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/httproutes/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "httproutes",
					Operation:   "Create/Update httproute",
					Description: "Create or update an httproute.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/httproutes/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "httproutes",
					Operation:   "Delete httproute",
					Description: "Delete an httproute.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/applications/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "applications",
					Operation:   "List applications",
					Description: "Get the list of applications.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/applications/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "applications",
					Operation:   "Create/Update application",
					Description: "Create or update an application.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/applications/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "applications",
					Operation:   "Delete application",
					Description: "Delete an application.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/gateways/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "gateways",
					Operation:   "List gateways",
					Description: "Get the list of gateways.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/gateways/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "gateways",
					Operation:   "Create/Update gateway",
					Description: "Create or Updateg a gateway.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/gateways/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "gateways",
					Operation:   "delete gateway",
					Description: "Delete a gateway.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/containers/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "containers",
					Operation:   "List containers",
					Description: "Get the list of containers.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/containers/write",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "containers",
					Operation:   "Create/Update container",
					Description: "Create or update a container.",
				},
				IsDataAction: false,
			},
			&v1.Operation{
				Name: "Applications.Core/containers/delete",
				Display: &v1.OperationDisplayProperties{
					Provider:    ProviderNamespaceName,
					Resource:    "containers",
					Operation:   "Delete container",
					Description: "Delete a container.",
				},
				IsDataAction: false,
			},
		},
	}
}
