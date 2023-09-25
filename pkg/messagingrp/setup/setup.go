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

	"github.com/radius-project/radius/pkg/armrpc/builder"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	frontend_ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel/converter"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	rmq_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller/rabbitmqqueues"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"
)

const (
	// AsyncOperationRetryAfter is polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// SetupNamespace builds the namespace for core resource provider.
func SetupNamespace(recipeControllerConfig *controllerconfig.RecipeControllerConfig) *builder.Namespace {
	ns := builder.NewNamespace("Applications.Messaging")

	_ = ns.AddResource("rabbitmqqueues", &builder.ResourceOption[*datamodel.RabbitMQQueue, datamodel.RabbitMQQueue]{
		RequestConverter:  converter.RabbitMQQueueDataModelFromVersioned,
		ResponseConverter: converter.RabbitMQQueueDataModelToVersioned,

		Put: builder.Operation[datamodel.RabbitMQQueue]{
			APIController: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  converter.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout:    msg_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		Patch: builder.Operation[datamodel.RabbitMQQueue]{
			APIController: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  converter.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout:    msg_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		Delete: builder.Operation[datamodel.RabbitMQQueue]{
			APIController: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:         converter.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:        converter.RabbitMQQueueDataModelToVersioned,
						AsyncOperationTimeout:    msg_ctrl.AsyncDeleteRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		List: builder.Operation[datamodel.RabbitMQQueue]{
			APIController: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:   converter.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:  converter.RabbitMQQueueDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
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
