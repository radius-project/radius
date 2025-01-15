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
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/builder"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
)

var handlerTests = []rpctest.HandlerTestSpec{
	{
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.datastores/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.MongoDatabasesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/mongodatabases/mongo/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.datastores/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.RedisCachesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/rediscaches/redis/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.datastores/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: ds_ctrl.SqlDatabasesResourceType, Method: ds_ctrl.OperationListSecret},
		Path:          "/resourcegroups/testrg/providers/applications.datastores/sqldatabases/sql/listsecrets",
		Method:        http.MethodPost,
	},
}

func TestRouter(t *testing.T) {
	cfg := &controllerconfig.RecipeControllerConfig{}
	ns := SetupNamespace(cfg)
	nsBuilder := ns.GenerateBuilder()

	rpctest.AssertRouters(t, handlerTests, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		validator, err := builder.NewOpenAPIValidator(ctx, "/api.ucp.dev", "applications.datastores")
		require.NoError(t, err)

		options := apictrl.Options{
			Address:        "localhost:9000",
			PathBase:       "/api.ucp.dev",
			DatabaseClient: inmemory.NewClient(),
			StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
		}

		return r, nsBuilder.ApplyAPIHandlers(ctx, r, options, validator)
	})
}
