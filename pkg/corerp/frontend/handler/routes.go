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

	// Used to register routes like:
	//
	// /planes/radius/{planeName}/providers/applications.core/environments
	planeScopeRouter := router.PathPrefix(pathBase).Subrouter()
	planeScopeRouter.Use(validator.APIValidator(specLoader))

	// Used to register routes like:
	//
	// /planes/radius/{planeName}/resourcegroups/{resourceGroupName}/providers/applications.core/environments
	resourceGroupScopeRouter := router.PathPrefix(pathBase + resourceGroupPath).Subrouter()
	resourceGroupScopeRouter.Use(validator.APIValidator(specLoader))

	// Adds environment resource type routes
	environmentPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/environments").Subrouter()
	environmentResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/environments").Subrouter()
	environmentResourceRouter := environmentResourceGroupRouter.Path("/{environmentName}").Subrouter()

	// Adds httproute resource type routes
	httpRoutePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/httproutes").Subrouter()
	httpRouteResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/httproutes").Subrouter()
	httpRouteResourceRouter := httpRouteResourceGroupRouter.Path("/{httpRouteName}").Subrouter()

	// Adds container resource type routes
	containerPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/containers").Subrouter()
	containerResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/containers").Subrouter()
	containerResourceRouter := containerResourceGroupRouter.Path("/{containerName}").Subrouter()

	// Adds application resource type routes
	applicationPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/applications").Subrouter()
	applicationResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/applications").Subrouter()
	applicationResourceRouter := applicationResourceGroupRouter.Path("/{applicationName}").Subrouter()

	// Adds gateway resource type routes
	gatewayPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/gateways").Subrouter()
	gatewayResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/gateways").Subrouter()
	gatewayResourceRouter := gatewayResourceGroupRouter.Path("/{gatewayName}").Subrouter()

	// Adds volume resource type routes
	volumePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/volumes").Subrouter()
	volumeResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/volumes").Subrouter()
	volumeResourceRouter := volumeResourceGroupRouter.Path("/{volumeName}").Subrouter()

	// Adds secretstore resource type routes
	secretStorePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.core/secretstores").Subrouter()
	secretStoreResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.core/secretstores").Subrouter()
	secretStoreResourceRouter := secretStoreResourceGroupRouter.Path("/{secretStoreName}").Subrouter()

	handlerOptions := []server.HandlerOptions{
		// Environments resource handler registration.
		{
			ParentRouter: environmentPlaneRouter,
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(
					opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter:  converter.EnvironmentDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: environmentResourceGroupRouter,
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
			ParentRouter: environmentResourceRouter,
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
			ParentRouter:   environmentResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:   environmentResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter: environmentResourceRouter,
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
			ParentRouter:   environmentResourceGroupRouter.Path("/{environmentName}/getmetadata").Subrouter(),
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         env_ctrl.OperationGetRecipeMetadata,
			HandlerFactory: env_ctrl.NewGetRecipeMetadata,
		},
		// httpRoute resource handler registration.
		{
			ParentRouter: httpRoutePlaneRouter,
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter:  converter.HTTPRouteDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: httpRouteResourceGroupRouter,
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
			ParentRouter: httpRouteResourceRouter,
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
			ParentRouter: httpRouteResourceRouter,
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
			ParentRouter: httpRouteResourceRouter,
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
			ParentRouter: httpRouteResourceRouter,
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
			ParentRouter: containerPlaneRouter,
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter:  converter.ContainerDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: containerResourceGroupRouter,
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
			ParentRouter: containerResourceRouter,
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
			ParentRouter: containerResourceRouter,
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
			ParentRouter: containerResourceRouter,
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
			ParentRouter: containerResourceRouter,
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
			ParentRouter: applicationPlaneRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter:  converter.ApplicationDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: applicationResourceGroupRouter,
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
			ParentRouter: applicationResourceRouter,
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
			ParentRouter: applicationResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						RequestConverter:  converter.ApplicationDataModelFromVersioned,
						ResponseConverter: converter.ApplicationDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Application]{
							rp_frontend.PrepareRadiusResource[*datamodel.Application],
							app_ctrl.CreateAppScopedNamespace,
						},
					},
				)
			},
		},
		{
			ParentRouter: applicationResourceRouter,
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						RequestConverter:  converter.ApplicationDataModelFromVersioned,
						ResponseConverter: converter.ApplicationDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Application]{
							rp_frontend.PrepareRadiusResource[*datamodel.Application],
							app_ctrl.CreateAppScopedNamespace,
						},
					},
				)
			},
		},
		{
			ParentRouter: applicationResourceRouter,
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
			ParentRouter: gatewayPlaneRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter:  converter.GatewayDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: gatewayResourceGroupRouter,
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
			ParentRouter: gatewayResourceRouter,
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
			ParentRouter: gatewayResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						RequestConverter:  converter.GatewayDataModelFromVersioned,
						ResponseConverter: converter.GatewayDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Gateway]{
							rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
							gtwy_ctrl.ValidateAndMutateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: gatewayResourceRouter,
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						RequestConverter:  converter.GatewayDataModelFromVersioned,
						ResponseConverter: converter.GatewayDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Gateway]{
							rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
							gtwy_ctrl.ValidateAndMutateRequest,
						},
					},
				)
			},
		},
		{
			ParentRouter: gatewayResourceRouter,
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
			ParentRouter: volumePlaneRouter,
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:   converter.VolumeResourceModelFromVersioned,
						ResponseConverter:  converter.VolumeResourceModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: volumeResourceGroupRouter,
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
			ParentRouter: volumeResourceRouter,
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
			ParentRouter: volumeResourceRouter,
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
			ParentRouter: volumeResourceRouter,
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
			ParentRouter: volumeResourceRouter,
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
			ParentRouter: secretStorePlaneRouter,
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter:  converter.SecretStoreModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceGroupRouter,
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
			ParentRouter: secretStoreResourceRouter,
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
			ParentRouter: secretStoreResourceRouter,
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
			ParentRouter: secretStoreResourceRouter,
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
			ParentRouter: secretStoreResourceRouter,
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
			ParentRouter:   secretStoreResourceGroupRouter.Path("/{secretStoreName}/listsecrets").Subrouter(),
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
