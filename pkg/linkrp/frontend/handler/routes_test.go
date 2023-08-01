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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/linkrp"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var handlerTests = []rpctest.HandlerTestSpec{
	// Routes for resources after Split Namespaces
	{
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.messaging/rabbitmqqueues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/pubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprPubSubBrokersResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/pubsubbrokers/daprpubsub",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprSecretStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/secretstores/daprsecretstore",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.dapr/statestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.N_DaprStateStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.dapr/statestores/daprstatestore",
		Method:        http.MethodDelete,
	},
	{
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/daprpubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/daprsecretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprsecretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/daprstatestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprstatestores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/extenders",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.ExtendersResourceType, Method: extender_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.link/extenders/extender/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: mongo_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: redis_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.link/rediscaches/redis/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/rabbitmqmessagequeues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList},
		Path:          "/providers/applications.link/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: sql_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationStatuses},
		Path:          "/providers/applications.link/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationResult},
		Path:          "/providers/applications.link/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationStatuses},
		Path:          "/providers/applications.messaging/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationResult},
		Path:          "/providers/applications.messaging/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	},
	{
		OperationType: v1.OperationType{Type: "Applications.Dapr/operationStatuses", Method: v1.OperationGetOperationStatuses},
		Path:          "/providers/applications.dapr/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Dapr/operationStatuses", Method: v1.OperationGetOperationResult},
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
				OperationType:               v1.OperationType{Type: "Applications.Link/providers", Method: v1.OperationGet},
				Path:                        "/providers/applications.link/operations",
				Method:                      http.MethodGet,
				WithoutRootScope:            true,
				SkipOperationTypeValidation: true,
			}, {
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
		}...)

		// Test handlers for Azure resources
		rpctest.AssertRouters(t, azureHandlerTests, "", "/subscriptions/00000000-0000-0000-0000-000000000000", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()
			return r, AddRoutes(ctx, r, true, ctrl.Options{PathBase: "", DataProvider: mockSP})
		})
	})
}
