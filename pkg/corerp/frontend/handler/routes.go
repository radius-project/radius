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

	"github.com/go-chi/chi/v5"

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

func AddRoutes(ctx context.Context, r chi.Router, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	rootScopePath := ctrlOpts.PathBase
	if isARM {
		rootScopePath += "/subscriptions/{subscriptionID}"
	} else {
		rootScopePath += "/planes/radius/{planeName}"
	}

	resourceGroupPath := "/resourcegroups/{resourceGroupName}"

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, r, rootScopePath, isARM, ProviderNamespaceName, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	// URLs may use either the subscription/plane scope or resource group scope.
	//
	// These paths are order sensitive and the longer path MUST be registered first.
	prefixes := []string{
		rootScopePath + resourceGroupPath,
		rootScopePath,
	}

	specLoader, err := validator.LoadSpec(ctx, ProviderNamespaceName, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	// Register Opena API validator middleware.
	router := server.NewSubrouter(r, rootScopePath)
	router.Use(validator.APIValidator(specLoader))

	handlerOptions := []server.HandlerOptions{
		// Environments resource handler registration.
		{
			ParentRouter: router,
			Path:         "/providers/applications.core/environments",
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(
					opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter:  converter.EnvironmentDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/environments",
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/environments/{environmentName}",
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:      router,
			Path:              resourceGroupPath + "/providers/applications.core/environments/{environmentName}",
			ResourceType:      env_ctrl.ResourceTypeName,
			Method:            v1.OperationPut,
			ControllerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:      router,
			Path:              resourceGroupPath + "/providers/applications.core/environments/{environmentName}",
			ResourceType:      env_ctrl.ResourceTypeName,
			Method:            v1.OperationPatch,
			ControllerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/environments/{environmentName}",
			ResourceType: env_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.Environment]{
						RequestConverter:  converter.EnvironmentDataModelFromVersioned,
						ResponseConverter: converter.EnvironmentDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:      router,
			Path:              resourceGroupPath + "/providers/applications.core/environments/{environmentName}/getmetadata",
			ResourceType:      env_ctrl.ResourceTypeName,
			Method:            env_ctrl.OperationGetRecipeMetadata,
			ControllerFactory: env_ctrl.NewGetRecipeMetadata,
		},
		// httpRoute resource handler registration.
		{
			ParentRouter: router,
			Path:         "/providers/applications.core/httproutes",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter:  converter.HTTPRouteDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/httproutes",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/httproutes/{httpRouteName}",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/httproutes/{httpRouteName}",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/httproutes/{httpRouteName}",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.HTTPRoute]{
						RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
						ResponseConverter: converter.HTTPRouteDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/httproutes/{httpRouteName}",
			ResourceType: hrt_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         "/providers/applications.core/containers",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter:  converter.ContainerDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/containers",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter: converter.ContainerDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/containers/{containerName}",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.ContainerResource]{
						ResponseConverter: converter.ContainerDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/containers/{containerName}",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/containers/{containerName}",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/containers/{containerName}",
			ResourceType: ctr_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         "/providers/applications.core/applications",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter:  converter.ApplicationDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/applications",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter: converter.ApplicationDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/applications/{applicationName}",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Application]{
						ResponseConverter: converter.ApplicationDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/applications/{applicationName}",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/applications/{applicationName}",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/applications/{applicationName}",
			ResourceType: app_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         "/providers/applications.core/gateways",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter:  converter.GatewayDataModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/gateways",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter: converter.GatewayDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/gateways/{gatewayName}",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Gateway]{
						ResponseConverter: converter.GatewayDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/gateways/{gatewayName}",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/gateways/{gatewayName}",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/gateways/{gatewayName}",
			ResourceType: gtwy_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         "/providers/applications.core/volumes",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/volumes",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/volumes/{volumeName}",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.VolumeResource]{
						RequestConverter:  converter.VolumeResourceModelFromVersioned,
						ResponseConverter: converter.VolumeResourceModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/volumes/{volumeName}",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/volumes/{volumeName}",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/volumes/{volumeName}",
			ResourceType: vol_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         "/providers/applications.core/secretstores",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter:  converter.SecretStoreModelToVersioned,
						ListRecursiveQuery: true,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/secretstores",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter: converter.SecretStoreModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/secretstores/{secretStoreName}",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.SecretStore]{
						ResponseConverter: converter.SecretStoreModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/secretstores/{secretStoreName}",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/secretstores/{secretStoreName}",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: router,
			Path:         resourceGroupPath + "/providers/applications.core/secretstores/{secretStoreName}",
			ResourceType: secret_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter:      router,
			Path:              resourceGroupPath + "/providers/applications.core/secretstores/{secretStoreName}/listsecrets",
			ResourceType:      secret_ctrl.ResourceTypeName,
			Method:            secret_ctrl.OperationListSecrets,
			ControllerFactory: secret_ctrl.NewListSecrets,
		},
	}
	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
