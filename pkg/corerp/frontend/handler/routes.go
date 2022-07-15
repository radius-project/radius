// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	app_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/gateway"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
)

const (
	ProviderNamespaceName = "Applications.Core"
)

func AddRoutes(ctx context.Context, router *mux.Router, pathBase string, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	} else {
		pathBase += "/planes/radius/{planeName}"
	}

	resourceGroupPath := "/resourcegroups/{resourceGroupName}"

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, ProviderNamespaceName, swagger.SpecFiles, pathBase+resourceGroupPath)
	if err != nil {
		return err
	}

	rootScopeRouter := router.PathPrefix(pathBase + resourceGroupPath).Subrouter()
	rootScopeRouter.Use(validator.APIValidator(specLoader))

	envRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/environments").Subrouter()
	envResourceRouter := envRTSubrouter.Path("/{environmentName}").Subrouter()

	hrtSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/httproutes").Subrouter()
	hrtResourceRouter := hrtSubrouter.Path("/{httpRouteName}").Subrouter()

	ctrRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/containers").Subrouter()
	ctrResourceRouter := ctrRTSubrouter.Path("/{containerName}").Subrouter()

	// Adds application resource type routes
	appRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/applications").Subrouter()
	appResourceRouter := appRTSubrouter.Path("/{applicationName}").Subrouter()

	// Adds gateway resource type routes
	gtwyRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/gateways").Subrouter()
	gtwyResourceRouter := gtwyRTSubrouter.Path("/{gatewayName}").Subrouter()

	handlerOptions := []server.HandlerOptions{
		// Environments resource handler registration.
		{
			ParentRouter:   envRTSubrouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: env_ctrl.NewListEnvironments,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: env_ctrl.NewGetEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: env_ctrl.NewDeleteEnvironment,
		},
		{
			ParentRouter:   hrtSubrouter,
			ResourceType:   hrt_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: hrt_ctrl.NewListHTTPRoutes,
		},
		{
			ParentRouter:   hrtResourceRouter,
			ResourceType:   hrt_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: hrt_ctrl.NewGetHTTPRoute,
		},
		{
			ParentRouter:   hrtResourceRouter,
			ResourceType:   hrt_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: hrt_ctrl.NewCreateOrUpdateHTTPRoute,
		},
		{
			ParentRouter:   hrtResourceRouter,
			ResourceType:   hrt_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: hrt_ctrl.NewCreateOrUpdateHTTPRoute,
		},
		{
			ParentRouter:   hrtResourceRouter,
			ResourceType:   hrt_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: hrt_ctrl.NewDeleteHTTPRoute,
		},
		// Container resource handlers
		{
			ParentRouter:   ctrRTSubrouter,
			ResourceType:   ctr_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: ctr_ctrl.NewListContainers,
		},
		{
			ParentRouter:   ctrResourceRouter,
			ResourceType:   ctr_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: ctr_ctrl.NewGetContainer,
		},
		{
			ParentRouter:   ctrResourceRouter,
			ResourceType:   ctr_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: ctr_ctrl.NewCreateOrUpdateContainer,
		},
		{
			ParentRouter:   ctrResourceRouter,
			ResourceType:   ctr_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: ctr_ctrl.NewCreateOrUpdateContainer,
		},
		{
			ParentRouter:   ctrResourceRouter,
			ResourceType:   ctr_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: ctr_ctrl.NewDeleteContainer,
		},
		// Applications resource handler registration.
		{
			ParentRouter:   appRTSubrouter,
			ResourceType:   app_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: app_ctrl.NewListApplications,
		},
		{
			ParentRouter:   appResourceRouter,
			ResourceType:   app_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: app_ctrl.NewGetApplication,
		},
		{
			ParentRouter:   appResourceRouter,
			ResourceType:   app_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: app_ctrl.NewCreateOrUpdateApplication,
		},
		{
			ParentRouter:   appResourceRouter,
			ResourceType:   app_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: app_ctrl.NewCreateOrUpdateApplication,
		},
		{
			ParentRouter:   appResourceRouter,
			ResourceType:   app_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: app_ctrl.NewDeleteApplication,
		},
		// Gateway resource handler registration.
		{
			ParentRouter:   gtwyRTSubrouter,
			ResourceType:   gtwy_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: gtwy_ctrl.NewListGateways,
		},
		{
			ParentRouter:   gtwyResourceRouter,
			ResourceType:   gtwy_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: gtwy_ctrl.NewGetGateway,
		},
		{
			ParentRouter:   gtwyResourceRouter,
			ResourceType:   gtwy_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: gtwy_ctrl.NewCreateOrUpdateGateway,
		},
		{
			ParentRouter:   gtwyResourceRouter,
			ResourceType:   gtwy_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: gtwy_ctrl.NewCreateOrUpdateGateway,
		},
		{
			ParentRouter:   gtwyResourceRouter,
			ResourceType:   gtwy_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: gtwy_ctrl.NewDeleteGateway,
		},
	}
	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
