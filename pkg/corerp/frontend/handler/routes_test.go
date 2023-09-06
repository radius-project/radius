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
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/store"

	app_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	hrt_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/httproutes"
	secret_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	vol_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/volumes"
)

var handlerTests = []rpctest.HandlerTestSpec{
	{
		OperationType: v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: env_ctrl.OperationGetRecipeMetadata},
		Path:          "/resourcegroups/testrg/providers/applications.core/environments/env0/getmetadata",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/providers/applications.core/httproutes",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.core/httproutes",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: secret_ctrl.OperationListSecrets},
		Path:          "/resourcegroups/testrg/providers/applications.core/secretstores/secret0/listsecrets",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationList},
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
		OperationType: v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGetOperationStatuses},
		Path:          "/providers/applications.core/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGetOperationResult},
		Path:          "/providers/applications.core/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	},
}

func TestHandlers(t *testing.T) {
	mctrl := gomock.NewController(t)

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSC := store.NewMockStorageClient(mctrl)
	mockEngine := engine.NewMockEngine(mctrl)

	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.Object{}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	t.Run("UCP", func(t *testing.T) {
		// Test handlers for UCP resources.
		rpctest.AssertRouters(t, handlerTests, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()
			return r, AddRoutes(ctx, r, false, ctrl.Options{PathBase: "/api.ucp.dev", DataProvider: mockSP}, mockEngine)
		})
	})

	t.Run("Azure", func(t *testing.T) {
		// Add azure specific handlers.
		azureHandlerTests := append(handlerTests,
			rpctest.HandlerTestSpec{
				OperationType:               v1.OperationType{Type: "Applications.Core/providers", Method: v1.OperationGet},
				Path:                        "/providers/applications.core/operations",
				Method:                      http.MethodGet,
				WithoutRootScope:            true,
				SkipOperationTypeValidation: true,
			})
		// Test handlers for Azure resources
		rpctest.AssertRouters(t, azureHandlerTests, "", "/subscriptions/00000000-0000-0000-0000-000000000000", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()
			return r, AddRoutes(ctx, r, true, ctrl.Options{PathBase: "", DataProvider: mockSP}, mockEngine)
		})
	})
}
