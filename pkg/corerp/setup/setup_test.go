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
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/database/inmemory"

	app_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	secret_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	vol_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/volumes"
)

var handlerTests = []rpctest.HandlerTestSpec{
	{
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/applications",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications/app0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications/app0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications/app0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications/app0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/containers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/containers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/environments",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: "ACTIONGETMETADATA"},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0/getmetadata",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/gateways",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/gateways",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: "ACTIONLISTSECRETS"},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.core/volumes",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/volumes",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGet},
		Path:          "/providers/applications.core/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGet},
		Path:          "/providers/applications.core/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: "ACTIONGETGRAPH"},
		Path:          "/resourcegroups/testrg/providers/applications.core/applications/app0/getgraph",
		Method:        http.MethodPost,
	},
}

func TestRouter(t *testing.T) {
	conn, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
	require.NoError(t, err)
	cfg := &controllerconfig.RecipeControllerConfig{
		UCPConnection: &conn,
	}
	ns := SetupNamespace(cfg)
	nsBuilder := ns.GenerateBuilder()

	rpctest.AssertRouters(t, handlerTests, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		validator, err := builder.NewOpenAPIValidator(ctx, "/api.ucp.dev", "applications.core")
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
