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

package setup

import (
	"time"

	asyncctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/builder"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	backend_ctrl "github.com/radius-project/radius/pkg/corerp/backend/controller"
	app_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/environments"
	ext_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/extenders"
	gw_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	secret_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	vol_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/volumes"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"

	ext_processor "github.com/radius-project/radius/pkg/corerp/processors/extenders"
	pr_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"
)

const (
	// AsyncOperationRetryAfter is polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// SetupNamespace builds the namespace for core resource provider.
func SetupNamespace(recipeControllerConfig *controllerconfig.RecipeControllerConfig) *builder.Namespace {
	ns := builder.NewNamespace("Applications.Core")

	_ = ns.AddResource("environments", &builder.ResourceOption[*datamodel.Environment, datamodel.Environment]{
		RequestConverter:  converter.EnvironmentDataModelFromVersioned,
		ResponseConverter: converter.EnvironmentDataModelToVersioned,

		Put: builder.Operation[datamodel.Environment]{
			APIController: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		Patch: builder.Operation[datamodel.Environment]{
			APIController: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		Custom: map[string]builder.Operation[datamodel.Environment]{
			"getmetadata": {
				APIController: func(opt apictrl.Options) (apictrl.Controller, error) {
					return env_ctrl.NewGetRecipeMetadata(opt, recipeControllerConfig.Engine)
				},
			},
		},
	})

	_ = ns.AddResource("applications", &builder.ResourceOption[*datamodel.Application, datamodel.Application]{
		RequestConverter:  converter.ApplicationDataModelFromVersioned,
		ResponseConverter: converter.ApplicationDataModelToVersioned,

		Put: builder.Operation[datamodel.Application]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Application]{
				rp_frontend.PrepareRadiusResource[*datamodel.Application],
				app_ctrl.CreateAppScopedNamespace,
			},
		},
		Patch: builder.Operation[datamodel.Application]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Application]{
				rp_frontend.PrepareRadiusResource[*datamodel.Application],
				app_ctrl.CreateAppScopedNamespace,
			},
		},
	})

	_ = ns.AddResource("httpRoutes", &builder.ResourceOption[*datamodel.HTTPRoute, datamodel.HTTPRoute]{
		RequestConverter:  converter.HTTPRouteDataModelFromVersioned,
		ResponseConverter: converter.HTTPRouteDataModelToVersioned,

		Put: builder.Operation[datamodel.HTTPRoute]{
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.HTTPRoute]{
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.HTTPRoute]{
			AsyncJobController:       backend_ctrl.NewDeleteResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("containers", &builder.ResourceOption[*datamodel.ContainerResource, datamodel.ContainerResource]{
		RequestConverter:  converter.ContainerDataModelFromVersioned,
		ResponseConverter: converter.ContainerDataModelToVersioned,

		Put: builder.Operation[datamodel.ContainerResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.ContainerResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
				ctr_ctrl.ValidateAndMutateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.ContainerResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.ContainerResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
				ctr_ctrl.ValidateAndMutateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.ContainerResource]{
			AsyncJobController:       backend_ctrl.NewDeleteResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("gateways", &builder.ResourceOption[*datamodel.Gateway, datamodel.Gateway]{
		RequestConverter:  converter.GatewayDataModelFromVersioned,
		ResponseConverter: converter.GatewayDataModelToVersioned,

		Put: builder.Operation[datamodel.Gateway]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Gateway]{
				rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
				gw_ctrl.ValidateAndMutateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.Gateway]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Gateway]{
				rp_frontend.PrepareRadiusResource[*datamodel.Gateway],
				gw_ctrl.ValidateAndMutateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.Gateway]{
			AsyncJobController:       backend_ctrl.NewDeleteResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("volumes", &builder.ResourceOption[*datamodel.VolumeResource, datamodel.VolumeResource]{
		RequestConverter:  converter.VolumeResourceModelFromVersioned,
		ResponseConverter: converter.VolumeResourceModelToVersioned,

		Put: builder.Operation[datamodel.VolumeResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.VolumeResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.VolumeResource],
				vol_ctrl.ValidateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.VolumeResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.VolumeResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.VolumeResource],
				vol_ctrl.ValidateRequest,
			},
			AsyncJobController:       backend_ctrl.NewCreateOrUpdateResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.VolumeResource]{
			AsyncJobController:       backend_ctrl.NewDeleteResource,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("secretStores", &builder.ResourceOption[*datamodel.SecretStore, datamodel.SecretStore]{
		RequestConverter:  converter.SecretStoreModelFromVersioned,
		ResponseConverter: converter.SecretStoreModelToVersioned,

		Put: builder.Operation[datamodel.SecretStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.SecretStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.SecretStore],
				secret_ctrl.ValidateAndMutateRequest,
				secret_ctrl.UpsertSecret,
			},
		},
		Patch: builder.Operation[datamodel.SecretStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.SecretStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.SecretStore],
				secret_ctrl.ValidateAndMutateRequest,
				secret_ctrl.UpsertSecret,
			},
		},
		Delete: builder.Operation[datamodel.SecretStore]{
			DeleteFilters: []apictrl.DeleteFilter[datamodel.SecretStore]{
				secret_ctrl.DeleteRadiusSecret,
			},
		},
		Custom: map[string]builder.Operation[datamodel.SecretStore]{
			"listsecrets": {
				APIController: secret_ctrl.NewListSecrets,
			},
		},
	})

	_ = ns.AddResource("extenders", &builder.ResourceOption[*datamodel.Extender, datamodel.Extender]{
		RequestConverter:  converter.ExtenderDataModelFromVersioned,
		ResponseConverter: converter.ExtenderDataModelToVersioned,

		Put: builder.Operation[datamodel.Extender]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Extender]{
				rp_frontend.PrepareRadiusResource[*datamodel.Extender],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource(options, &ext_processor.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.Extender]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.Extender]{
				rp_frontend.PrepareRadiusResource[*datamodel.Extender],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource(options, &ext_processor.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.Extender]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource(options, &ext_processor.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Custom: map[string]builder.Operation[datamodel.Extender]{
			"listsecrets": {
				APIController: ext_ctrl.NewListSecretsExtender,
			},
		},
	})

	// Optional
	ns.SetAvailableOperations(operationList)

	return ns
}
