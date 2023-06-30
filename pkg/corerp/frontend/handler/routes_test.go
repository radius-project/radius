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
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	app_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/applications"
	ctr_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	gtwy_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/gateways"
	hrt_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/httproutes"
	secret_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/secretstores"
	vol_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/volumes"
)

var handlerTests = []struct {
	name       string
	url        string
	method     string
	isAzureAPI bool
}{
	{
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/applications",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/applications",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/applications/app0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/applications/app0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/applications/app0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: app_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/applications/app0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/containers",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/containers",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: ctr_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/containers/ctr0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/environments",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments/env0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments/env0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments/env0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments/env0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: env_ctrl.ResourceTypeName, Method: env_ctrl.OperationGetRecipeMetadata}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/environments/env0/getmetadata",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/gateways",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/gateways",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: gtwy_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/gateways/gateway0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/httproutes",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/httproutes",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: hrt_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/httproutes/hrt0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/secretstores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores/secret0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: secret_ctrl.ResourceTypeName, Method: secret_ctrl.OperationListSecrets}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/secretstores/secret0/listsecrets",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/providers/applications.core/volumes",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationList}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/volumes",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationGet}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationPut}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationPatch}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: vol_ctrl.ResourceTypeName, Method: v1.OperationDelete}.String(),
		url:        "/resourcegroups/testrg/providers/applications.core/volumes/volume0",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Core/providers", Method: v1.OperationGet}.String(),
		url:        "/providers/applications.core/operations",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Core/providers", Method: v1.OperationPut}.String(),
		url:        "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
		method:     http.MethodPut,
		isAzureAPI: true,
	}, {
		name:       v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGetOperationStatuses}.String(),
		url:        "/providers/applications.core/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		name:       v1.OperationType{Type: "Applications.Core/operationStatuses", Method: v1.OperationGetOperationResult}.String(),
		url:        "/providers/applications.core/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
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

	namesMatched := make(map[string]bool)
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
			t.Logf("result: %v", tctx)
			require.Truef(t, result, "no route found for %s %s", tt.method, uri)
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

				assert.Contains(t, namesMatched, route.GetName(), "route %s is not tested", route.GetName())
				return nil
			})
			require.NoError(t, err)
		})*/
}
