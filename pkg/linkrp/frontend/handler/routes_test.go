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
	"github.com/project-radius/radius/pkg/linkrp"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
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
		url:        "/providers/applications.messaging/rabbitmqqueues",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.N_RabbitMQQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprpubsubbrokers",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprPubSubBrokersResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprsecretstores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprSecretStoresResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/daprstatestores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.DaprStateStoresResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/extenders",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.ExtendersResourceType, Method: extender_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/mongodatabases",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.MongoDatabasesResourceType, Method: mongo_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/rediscaches",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RedisCachesResourceType, Method: redis_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/rabbitmqmessagequeues",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.RabbitMQMessageQueuesResourceType, Method: rabbitmq_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/providers/applications.link/sqldatabases",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: linkrp.SqlDatabasesResourceType, Method: sql_ctrl.OperationListSecret}.String(),
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/providers", Method: v1.OperationGet}.String(),
		url:        "/providers/applications.link/operations",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationStatuses}.String(),
		url:        "/providers/applications.link/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Link/operationStatuses", Method: v1.OperationGetOperationResult}.String(),
		url:        "/providers/applications.link/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/providers", Method: v1.OperationGet}.String(),
		url:        "/providers/applications.messaging/operations",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationStatuses}.String(),
		url:        "/providers/applications.messaging/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Messaging/operationStatuses", Method: v1.OperationGetOperationResult}.String(),
		url:        "/providers/applications.messaging/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
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
	r := chi.NewRouter()
	err := AddRoutes(context.Background(), r, isARM, ctrl.Options{PathBase: pathBase, DataProvider: mockSP})
	require.NoError(t, err)

	namesMatched := map[string]bool{}
	for _, tt := range handlerTests {
		if !isARM && tt.isAzureAPI {
			namesMatched[tt.name] = true
			continue
		}

		uri := pathBase + "/planes/radius/{planeName}" + tt.url
		if isARM {
			if tt.isAzureAPI {
				uri = pathBase + tt.url
			} else {
				uri = pathBase + "/subscriptions/00000000-0000-0000-0000-000000000000" + tt.url
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			tctx := chi.NewRouteContext()
			tctx.Reset()

			result := r.Match(tctx, tt.method, uri)
			require.Truef(t, result, "no route found for %s %s, context: %v", tt.method, uri, tctx)
		})
	}

	t.Run("all named routes are tested", func(t *testing.T) {
		err := chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			t.Logf("%s %s", method, route)
			return nil
		})
		require.NoError(t, err)
	})
	/*
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
		})*/
}
