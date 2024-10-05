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
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/daprrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	bindings_proc "github.com/radius-project/radius/pkg/daprrp/processors/bindings"
	configurationstores_proc "github.com/radius-project/radius/pkg/daprrp/processors/configurationstores"
	pubsub_proc "github.com/radius-project/radius/pkg/daprrp/processors/pubsubbrokers"
	secretstore_proc "github.com/radius-project/radius/pkg/daprrp/processors/secretstores"
	statestore_proc "github.com/radius-project/radius/pkg/daprrp/processors/statestores"
	pr_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"
)

const (
	// AsyncOperationRetryAfter is polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// SetupNamespace builds the namespace for dapr resource provider.
func SetupNamespace(recipeControllerConfig *controllerconfig.RecipeControllerConfig) *builder.Namespace {
	ns := builder.NewNamespace("Applications.Dapr")

	_ = ns.AddResource("pubSubBrokers", &builder.ResourceOption[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker]{
		RequestConverter:  converter.PubSubBrokerDataModelFromVersioned,
		ResponseConverter: converter.PubSubBrokerDataModelToVersioned,

		Put: builder.Operation[datamodel.DaprPubSubBroker]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprPubSubBroker]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprPubSubBroker],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker](options, &pubsub_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.DaprPubSubBroker]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprPubSubBroker]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprPubSubBroker],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker](options, &pubsub_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.DaprPubSubBroker]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker](options, &pubsub_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprPubSubBrokerTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("stateStores", &builder.ResourceOption[*datamodel.DaprStateStore, datamodel.DaprStateStore]{
		RequestConverter:  converter.StateStoreDataModelFromVersioned,
		ResponseConverter: converter.StateStoreDataModelToVersioned,

		Put: builder.Operation[datamodel.DaprStateStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprStateStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprStateStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](options, &statestore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.DaprStateStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprStateStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprStateStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](options, &statestore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.DaprStateStore]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](options, &statestore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprStateStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("secretStores", &builder.ResourceOption[*datamodel.DaprSecretStore, datamodel.DaprSecretStore]{
		RequestConverter:  converter.SecretStoreDataModelFromVersioned,
		ResponseConverter: converter.SecretStoreDataModelToVersioned,

		Put: builder.Operation[datamodel.DaprSecretStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprSecretStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprSecretStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprSecretStore, datamodel.DaprSecretStore](options, &secretstore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.DaprSecretStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprSecretStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprSecretStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprSecretStore, datamodel.DaprSecretStore](options, &secretstore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.DaprSecretStore]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.DaprSecretStore, datamodel.DaprSecretStore](options, &secretstore_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprSecretStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("configurationStores", &builder.ResourceOption[*datamodel.DaprConfigurationStore, datamodel.DaprConfigurationStore]{
		RequestConverter:  converter.ConfigurationStoreDataModelFromVersioned,
		ResponseConverter: converter.ConfigurationStoreDataModelToVersioned,

		Put: builder.Operation[datamodel.DaprConfigurationStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprConfigurationStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprConfigurationStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprConfigurationStore, datamodel.DaprConfigurationStore](options, &configurationstores_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprConfigurationStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.DaprConfigurationStore]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprConfigurationStore]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprConfigurationStore],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprConfigurationStore, datamodel.DaprConfigurationStore](options, &configurationstores_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprConfigurationStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.DaprConfigurationStore]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.DaprConfigurationStore, datamodel.DaprConfigurationStore](options, &configurationstores_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprConfigurationStoreTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	_ = ns.AddResource("bindings", &builder.ResourceOption[*datamodel.DaprBinding, datamodel.DaprBinding]{
		RequestConverter:  converter.BindingDataModelFromVersioned,
		ResponseConverter: converter.BindingDataModelToVersioned,

		Put: builder.Operation[datamodel.DaprBinding]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprBinding]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprBinding],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprBinding, datamodel.DaprBinding](options, &bindings_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprBindingTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.DaprBinding]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.DaprBinding]{
				rp_frontend.PrepareRadiusResource[*datamodel.DaprBinding],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.DaprBinding, datamodel.DaprBinding](options, &bindings_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprBindingTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.DaprBinding]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.DaprBinding, datamodel.DaprBinding](options, &bindings_proc.Processor{Client: options.KubeClient}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprBindingTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
	})

	// Optional
	ns.SetAvailableOperations(operationList)

	return ns
}
