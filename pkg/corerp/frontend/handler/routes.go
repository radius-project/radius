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
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
)

const (
	ProviderNamespaceName = "Applications.Core"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, router *mux.Router, pathBase string) error {
	root := router
	if pathBase != "" {
		root = router.PathPrefix(pathBase).Subrouter()
	}

	subscriptionRt := router
	if !hostoptions.IsSelfHosted() {
		subscriptionRt = router.PathPrefix(pathBase + "/subscriptions/{subscriptionID}").Subrouter()
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, root, subscriptionRt, ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	envRTSubrouter := subscriptionRt.PathPrefix("/resourcegroups/{resourceGroup}/providers/applications.core/environments").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.PathPrefix("/{environment}").Subrouter()

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
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
