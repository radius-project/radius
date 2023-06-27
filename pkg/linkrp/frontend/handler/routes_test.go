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

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var handlerTests = []struct {
	name       string
	url        string
	method     string
	isAzureAPI bool
}{
	// Routes for resources after Split Namespaces
	{
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.messaging/rabbitmqqueues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/extenders?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: extender_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: mongo_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: redis_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/rabbitmqmessagequeues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/sqldatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: sql_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/providers", Method: v1.OperationGet}.String(),
		url:        "/providers/applications.link/operations?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/providers", Method: v1.OperationPut}.String(),
		url:        "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
		method:     http.MethodPut,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationStatuses}.String(),
		url:        "/providers/applications.link/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationResult}.String(),
		url:        "/providers/applications.link/locations/global/operationresults/00000000-0000-0000-0000-000000000000?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/providers", Method: v1.OperationGet}.String(),
		url:        "/providers/applications.messaging/operations?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationStatuses}.String(),
		url:        "/providers/applications.messaging/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationResult}.String(),
		url:        "/providers/applications.messaging/locations/global/operationresults/00000000-0000-0000-0000-000000000000?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
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

	assertRouters(t, "", true, mockSP)
	assertRouters(t, "/api.ucp.dev", false, mockSP)
}

func assertRouters(t *testing.T, pathBase string, isARM bool, mockSP *dataprovider.MockDataStorageProvider) {
	r := mux.NewRouter()
	err := AddRoutes(context.Background(), r, isARM, ctrl.Options{PathBase: pathBase, DataProvider: mockSP})
	require.NoError(t, err)

	namesMatched := map[string]bool{}
	for _, tt := range handlerTests {
		if !isARM && tt.isAzureAPI {
			namesMatched[tt.name] = true
			continue
		}

		uri := "http://localhost" + pathBase + "/planes/radius/{planeName}" + tt.url
		if isARM {
			if tt.isAzureAPI {
				uri = "http://localhost" + pathBase + tt.url
			} else {
				uri = "http://localhost" + pathBase + "/subscriptions/00000000-0000-0000-0000-000000000000" + tt.url
			}
		}
		if !isARM {
			uri = "http://localhost" + pathBase + "/planes/radius/local" + tt.url
		}

		t.Run(uri, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), tt.method, uri, nil)
			require.NoError(t, err)
			var match mux.RouteMatch
			require.True(t, r.Match(req, &match), "no route found for %s", uri)
			require.NoError(t, match.MatchErr, "route match error for %s", uri)

			require.Equal(t, tt.name, match.Route.GetName())
			if match.Route.GetName() != "" {
				namesMatched[match.Route.GetName()] = true
			}
		})
	}

	t.Run("all named routes are tested", func(t *testing.T) {
		err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
			if route.GetName() == "" {
				return nil
			}

			pathTemplate, err := route.GetPathTemplate()
			require.NoError(t, err)

			// Skip over the subscription registration route. This RP registers two routes for the same path.
			if pathTemplate == "/subscriptions/{subscriptionID}" {
				return nil
			}

			assert.Contains(t, namesMatched, route.GetName(), "route %s is not tested", route.GetName())
			return nil
		})
		require.NoError(t, err)
	})
}
