// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
)

const (
	ProviderNamespaceName = "Applications.Core"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, router *mux.Router, pathBase string, isARM bool) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	envRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/environments").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.PathPrefix("/{environment}").Subrouter()

	hrtSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.core/httproutes").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	hrtResourceRouter := hrtSubrouter.PathPrefix("/{httproute}").Subrouter()

	handlerOptions := []server.HandlerOptions{
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
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
