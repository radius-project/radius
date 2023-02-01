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
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	converter "github.com/project-radius/radius/pkg/corerp/datamodel/converter"

	app_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/gateway"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
	vol_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/volumes"
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

	specLoader, err := validator.LoadSpec(ctx, ProviderNamespaceName, swagger.SpecFiles, pathBase+resourceGroupPath, "rootScope")
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

	// Adds volume resource type routes
	volRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/volumes").Subrouter()
	volResourceRouter := volRTSubrouter.Path("/{volumeName}").Subrouter()

	handlerOptions := []server.HandlerOptions{
		// Environments resource handler registration.
		{
			ParentRouter: envRTSubrouter,
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: envResourceRouter,
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: hrtSubrouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: hrtResourceRouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: ctrRTSubrouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter: converter.ContainerDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: ctrResourceRouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter: converter.ContainerDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: appRTSubrouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter: converter.ApplicationDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: appResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter: converter.ApplicationDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: gtwyRTSubrouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter: converter.GatewayDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: gtwyResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter: converter.GatewayDataModelToVersioned,
					},
				)
			},
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
		// Volumes resource handler registration.
		{
			ParentRouter: volRTSubrouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: volResourceRouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: volResourceRouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.VolumeResource]{
							rp_frontend.PrepareRadiusResource[*datamodel.VolumeResource],
							vol_ctrl.ValidateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: volResourceRouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.VolumeResource]{
							rp_frontend.PrepareRadiusResource[*datamodel.VolumeResource],
							vol_ctrl.ValidateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: volResourceRouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:   envResourceRouter.PathPrefix("/getrecipedetails/{recipeName}").Subrouter(),
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         env_ctrl.OperationGetRecipeDetails,
			HandlerFactory: env_ctrl.NewGetRecipeDetails,
		},
	}
	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
