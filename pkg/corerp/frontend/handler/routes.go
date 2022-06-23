// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	app_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/gateway"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
)

const (
	ProviderNamespaceName = "Applications.Core"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, sm manager.StatusManager, router *mux.Router, pathBase string, isARM bool) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	} else {
		pathBase += "/planes/radius/{planeName}"
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, sm, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	envRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/environments").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.PathPrefix("/{environment}").Subrouter()

	hrtSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/httproutes").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	hrtResourceRouter := hrtSubrouter.PathPrefix("/{httproute}").Subrouter()

	ctrRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/containers").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	ctrResourceRouter := ctrRTSubrouter.PathPrefix("/{container}").Subrouter()

	// Adds application resource type routes
	appRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/applications").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	appResourceRouter := appRTSubrouter.PathPrefix("/{application}").Subrouter()

	// Adds gateway resource type routes
	gtwyRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/gateways").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	gtwyResourceRouter := gtwyRTSubrouter.PathPrefix("/{application}").Subrouter()

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
			ParentRouter:   ctrRTSubrouter,
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
		// TODO: Add async registration for createorupdate and delete handler
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
	}
	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, sm, h); err != nil {
			return err
		}
	}

	return nil
}
