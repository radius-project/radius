// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"context"
	"fmt"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	kubernetes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/kubernetes"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	resourcegroups_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"
)

// TODO: Use variables and construct the path as we add more APIs.
const (
	planeCollectionPath   = "/planes"
	planeItemPath         = "/planes/{PlaneType}/{PlaneID}"
	planeCollectionByType = "/planes/{PlaneType}"
)

var resourceGroupCollectionPath = fmt.Sprintf("%s/%s", planeItemPath, "resource{[gG]}roups")
var resourceGroupItemPath = fmt.Sprintf("%s/%s", resourceGroupCollectionPath, "{ResourceGroup}")

// Register registers the routes for UCP
func Register(ctx context.Context, router *mux.Router, ctrlOpts ctrl.Options) error {
	baseURL := ctrlOpts.BasePath

	handlerOptions := []ctrl.HandlerOptions{}

	// If we're in Kubernetes we have some required routes to implement.
	if baseURL != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.
		handlerOptions = append(handlerOptions, []ctrl.HandlerOptions{
			{
				ParentRouter:   router.Path("/openapi/v2").Subrouter(),
				Method:         v1.OperationGet,
				HandlerFactory: kubernetes_ctrl.NewOpenAPIv2Doc,
			},
			{
				ParentRouter:   router.Path(baseURL).Subrouter(),
				Method:         v1.OperationGet,
				HandlerFactory: kubernetes_ctrl.NewDiscoveryDoc,
			},
		}...)

	}
	logger := ucplog.GetLogger(ctx)
	logger.Info(fmt.Sprintf("UCP base path: %s", baseURL))

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, baseURL+planeCollectionPath)
	if err != nil {
		return err
	}

	subrouter := router.PathPrefix(baseURL + planeCollectionPath).Subrouter()
	subrouter.Use(validator.APIValidatorUCP(specLoader))

	rootScopeRouter := router.PathPrefix(baseURL + planeCollectionPath).Subrouter()
	rootScopeRouter.Use(validator.APIValidatorUCP(specLoader))
	ctrl.ConfigureDefaultHandlers(router, ctrl.Options{
		BasePath: baseURL,
	})

	planeCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeCollectionPath)).Subrouter()
	planeCollectionByTypeSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeCollectionByType)).Subrouter()
	planeSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeItemPath)).Subrouter()

	resourceGroupCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, resourceGroupCollectionPath)).Subrouter()
	resourceGroupSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, resourceGroupItemPath)).Subrouter()

	handlerOptions = append(handlerOptions, []ctrl.HandlerOptions{
		// Planes resource handler registration.
		{
			ParentRouter:   planeCollectionSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: planes_ctrl.NewListPlanes,
		},
		{
			ParentRouter:   planeCollectionByTypeSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: planes_ctrl.NewListPlanes,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: planes_ctrl.NewGetPlane,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationPut,
			HandlerFactory: planes_ctrl.NewCreateOrUpdatePlane,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationDelete,
			HandlerFactory: planes_ctrl.NewDeletePlane,
		},
		// Resource group handler registration
		{
			ParentRouter:   resourceGroupCollectionSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: resourcegroups_ctrl.NewListResourceGroups,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: resourcegroups_ctrl.NewGetResourceGroup,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationPut,
			HandlerFactory: resourcegroups_ctrl.NewCreateOrUpdateResourceGroup,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationDelete,
			HandlerFactory: resourcegroups_ctrl.NewDeleteResourceGroup,
		},

		// Proxy request should take the least priority in routing and should therefore be last
		{
			ParentRouter:   router,
			Path:           fmt.Sprintf("%s%s", baseURL, planeItemPath),
			HandlerFactory: planes_ctrl.NewProxyPlane,
		},
	}...)

	for _, h := range handlerOptions {
		if err := ctrl.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
