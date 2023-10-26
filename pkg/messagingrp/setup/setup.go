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
	"github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	msrp_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	rmq_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller/rabbitmqqueues"
	rmq_proc "github.com/radius-project/radius/pkg/messagingrp/processors/rabbitmqqueues"
	pr_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"
)

const (
	// AsyncOperationRetryAfter is polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// SetupNamespace builds the namespace for datastores resource provider.
func SetupNamespace(recipeControllerConfig *controllerconfig.RecipeControllerConfig) *builder.Namespace {
	ns := builder.NewNamespace("Applications.Messaging")

	_ = ns.AddResource("rabbitMQQueues", &builder.ResourceOption[*datamodel.RabbitMQQueue, datamodel.RabbitMQQueue]{
		RequestConverter:  converter.RabbitMQQueueDataModelFromVersioned,
		ResponseConverter: converter.RabbitMQQueueDataModelToVersioned,

		Put: builder.Operation[datamodel.RabbitMQQueue]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.RabbitMQQueue]{
				rp_frontend.PrepareRadiusResource[*datamodel.RabbitMQQueue],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.RabbitMQQueue, datamodel.RabbitMQQueue](options, &rmq_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    msrp_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.RabbitMQQueue]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.RabbitMQQueue]{
				rp_frontend.PrepareRadiusResource[*datamodel.RabbitMQQueue],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.RabbitMQQueue, datamodel.RabbitMQQueue](options, &rmq_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    msrp_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.RabbitMQQueue]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.RabbitMQQueue, datamodel.RabbitMQQueue](options, &rmq_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    msrp_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Custom: map[string]builder.Operation[datamodel.RabbitMQQueue]{
			"listsecrets": {
				APIController: rmq_ctrl.NewListSecretsRabbitMQQueue,
			},
		},
	})

	// Optional
	ns.SetAvailableOperations(operationList)

	return ns
}
