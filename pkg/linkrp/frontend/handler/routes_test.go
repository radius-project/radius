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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

var handlerOldTests = []struct {
	url        string
	method     string
	isAzureAPI bool
}{
	{
		url:        "/providers/applications.link/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/mongodatabases/mongo/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/operations?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		url:        "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
		method:     http.MethodPut,
		isAzureAPI: true,
	}, {
		url:        "/providers/applications.link/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rediscaches/redis/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/rabbitmqmessagequeues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/rabbitmqmessagequeues/rabbitmq/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/sqldatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/extenders?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/extenders?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/extenders/extender/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.link/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.link/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	},
	// Split Namespaces

	{
		url:        "/providers/applications.datastores/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.datastores/operations?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		url:        "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
		method:     http.MethodPut,
		isAzureAPI: true,
	}, {
		url:        "/providers/applications.datastores/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.messaging/rabbitmqqueues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	},
	{
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.messaging/rabbitmqqueues/rabbitmq/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.datastores/sqldatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.dapr/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprstatestores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprstatestores/daprstatestore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.dapr/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprsecretstores?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprsecretstores/daprsecretstore?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.dapr/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprpubsubbrokers?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.dapr/daprpubsubbrokers/daprpubsub?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	},
}

func TestOldHandlers(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSC := store.NewMockStorageClient(mctrl)

	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.Object{}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	assertOldRouters(t, "", true, mockSP)
	assertOldRouters(t, "/api.ucp.dev", false, mockSP)
}

func assertOldRouters(t *testing.T, pathBase string, isARM bool, mockSP *dataprovider.MockDataStorageProvider) {
	r := mux.NewRouter()
	err := AddRoutes(context.Background(), r, isARM, ctrl.Options{PathBase: pathBase, DataProvider: mockSP})
	require.NoError(t, err)

	for _, tt := range handlerOldTests {
		if !isARM && tt.isAzureAPI {
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
			req, _ := http.NewRequestWithContext(context.Background(), tt.method, uri, nil)
			var match mux.RouteMatch
			require.True(t, r.Match(req, &match), "no route found for %s", uri)
		})
	}
}
