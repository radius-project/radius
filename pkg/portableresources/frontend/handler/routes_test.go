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
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	dapr_types "github.com/radius-project/radius/pkg/daprrp"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	rabbitmq_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller/rabbitmqqueues"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/store"
)

var handlerTests = []rpctest.HandlerTestSpec{
	{
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.messaging/rabbitmqqueues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RabbitMQQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/pubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprPubSubBrokersResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprSecretStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/statestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: dapr_types.DaprStateStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.datastores/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: portableresources.MongoDatabasesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.datastores/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: portableresources.RedisCachesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.datastores/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: portableresources.SqlDatabasesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGet},
		Path:          "/providers/applications.messaging/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Messaging/operationResults", Method: v1.OperationGet},
		Path:          "/providers/applications.messaging/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	},
	{
		OperationType: v1.OperationType{Type: "Applications.Dapr/operationStatuses", Method: v1.OperationGet},
		Path:          "/providers/applications.dapr/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Dapr/operationResults", Method: v1.OperationGet},
		Path:          "/providers/applications.dapr/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	},
}

func TestHandlers(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSC := store.NewMockStorageClient(mctrl)

	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.Object{}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	t.Run("UCP", func(t *testing.T) {
		// Test handlers for UCP resources.
		rpctest.AssertRouters(t, handlerTests, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()
			return r, AddRoutes(ctx, r, false, ctrl.Options{PathBase: "/api.ucp.dev", DataProvider: mockSP})
		})
	})

	t.Run("Azure", func(t *testing.T) {
		// Add azure specific handlers.
		azureHandlerTests := append(handlerTests, []rpctest.HandlerTestSpec{
			{
				OperationType:               v1.OperationType{Type: "Applications.Messaging/providers", Method: v1.OperationGet},
				Path:                        "/providers/applications.messaging/operations",
				Method:                      http.MethodGet,
				WithoutRootScope:            true,
				SkipOperationTypeValidation: true,
			},
			{
				OperationType:               v1.OperationType{Type: "Applications.Dapr/providers", Method: v1.OperationGet},
				Path:                        "/providers/applications.dapr/operations",
				Method:                      http.MethodGet,
				WithoutRootScope:            true,
				SkipOperationTypeValidation: true,
			},
			{
				OperationType:               v1.OperationType{Type: "Applications.Datastores/providers", Method: v1.OperationGet},
				Path:                        "/providers/applications.datastores/operations",
				Method:                      http.MethodGet,
				WithoutRootScope:            true,
				SkipOperationTypeValidation: true,
			},
		}...)

		// Test handlers for Azure resources
		rpctest.AssertRouters(t, azureHandlerTests, "", "/subscriptions/00000000-0000-0000-0000-000000000000", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()
			return r, AddRoutes(ctx, r, true, ctrl.Options{PathBase: "", DataProvider: mockSP})
		})
	})
}
