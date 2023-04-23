/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	gtwy_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/gateways"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
	secret_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/secretstores"
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

	// Adds secretstore resource type routes
	secretRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.core/secretstores").Subrouter()
	secretResourceRouter := secretRTSubrouter.Path("/{secretStoreName}").Subrouter()

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
			ParentRouter: envResourceRouter,
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						RequestConverter:  converter.EnvironmentDataModelFromVersioned,
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:   envRTSubrouter.Path("/{environmentName}/getmetadata").Subrouter(),
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         env_ctrl.OperationGetRecipeMetadata,
			HandlerFactory: env_ctrl.NewGetRecipeMetadata,
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
			ParentRouter: hrtResourceRouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: hrtResourceRouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: hrtResourceRouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: ctrResourceRouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						RequestConverter:  converter.ContainerDataModelFromVersioned,
						ResponseConverter: converter.ContainerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.ContainerResource]{
							rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
							ctr_ctrl.ValidateAndMutateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: ctrResourceRouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						RequestConverter:  converter.ContainerDataModelFromVersioned,
						ResponseConverter: converter.ContainerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.ContainerResource]{
							rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
							ctr_ctrl.ValidateAndMutateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: ctrResourceRouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						RequestConverter:  converter.ContainerDataModelFromVersioned,
						ResponseConverter: converter.ContainerDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: appResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						RequestConverter:  converter.ApplicationDataModelFromVersioned,
						ResponseConverter: converter.ApplicationDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Application]{
							rp_frontend.PrepareRadiusResource[*datamodel.Application],
							app_ctrl.UpsertAppKubernetesNamespace,
						},
					},
				)
			},
		},
		{
			ParentRouter: appResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						RequestConverter:  converter.ApplicationDataModelFromVersioned,
						ResponseConverter: converter.ApplicationDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Application]{
							rp_frontend.PrepareRadiusResource[*datamodel.Application],
							app_ctrl.UpsertAppKubernetesNamespace,
						},
					},
				)
			},
		},
		{
			ParentRouter: appResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						RequestConverter:  converter.ApplicationDataModelFromVersioned,
						ResponseConverter: converter.ApplicationDataModelToVersioned,
					},
				)
			},
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
			ParentRouter: gtwyResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						RequestConverter:  converter.GatewayDataModelFromVersioned,
						ResponseConverter: converter.GatewayDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Gateway]{
							rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
							gtwy_ctrl.ValidateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: gtwyResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						RequestConverter:  converter.GatewayDataModelFromVersioned,
						ResponseConverter: converter.GatewayDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Gateway]{
							rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
							gtwy_ctrl.ValidateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: gtwyResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						RequestConverter:  converter.GatewayDataModelFromVersioned,
						ResponseConverter: converter.GatewayDataModelToVersioned,
					},
				)
			},
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
		// Secret Store resource handler registration.
		{
			ParentRouter: secretRTSubrouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter: converter.SecretStoreModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: secretResourceRouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter: converter.SecretStoreModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: secretResourceRouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						RequestConverter:  converter.SecretStoreModelFromVersioned,
						ResponseConverter: converter.SecretStoreModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.SecretStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.SecretStore],
							secret_ctrl.ValidateAndMutateRequest,
							secret_ctrl.UpsertSecret,
						},
					},
				)
			},
		},
		{
			ParentRouter: secretResourceRouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						RequestConverter:  converter.SecretStoreModelFromVersioned,
						ResponseConverter: converter.SecretStoreModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.SecretStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.SecretStore],
							secret_ctrl.ValidateAndMutateRequest,
							secret_ctrl.UpsertSecret,
						},
					},
				)
			},
		},
		{
			ParentRouter: secretResourceRouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter: converter.SecretStoreModelToVersioned,
						DeleteFilters: []frontend_ctrl.DeleteFilter[datamodel.SecretStore]{
							secret_ctrl.DeleteRadiusSecret,
						},
					},
				)
			},
		},
		{
			ParentRouter:   secretRTSubrouter.Path("/{secretStoreName}/listsecrets").Subrouter(),
			ResourceType:   secret_ctrl.ResourceTypeName,
			Method:         secret_ctrl.OperationListSecrets,
			HandlerFactory: secret_ctrl.NewListSecrets,
		},
	}
	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
