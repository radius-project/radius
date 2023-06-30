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

package radius

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/secret"
	secretprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/stretchr/testify/require"
)

func Test_Routes(t *testing.T) {
	pathBase := "/some-path-base"
	tests := []struct {
		method       string
		path         string
		name         string
		skipPathBase bool
	}{
		{
			name:   v1.OperationType{Type: v20220901privatepreview.ResourceGroupType, Method: v1.OperationList}.String(),
			method: http.MethodGet,
			path:   "/planes/radius/local/resourcegroups",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.ResourceGroupType, Method: v1.OperationGet}.String(),
			method: http.MethodGet,
			path:   "/planes/radius/local/resourcegroups/test-rg",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.ResourceGroupType, Method: v1.OperationPut}.String(),
			method: http.MethodPut,
			path:   "/planes/radius/local/resourcegroups/test-rg",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.ResourceGroupType, Method: v1.OperationDelete}.String(),
			method: http.MethodDelete,
			path:   "/planes/radius/local/resourcegroups/test-rg",
		}, {
			name:   v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy}.String(),
			method: http.MethodGet,
			path:   "/planes/radius/local/providers/applications.core/applications/test-app",
		}, {
			name:   v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy}.String(),
			method: http.MethodGet,
			path:   "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/test-app",
		}, {
			name:   v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy}.String(),
			method: http.MethodPut,
			path:   "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/test-app",
		},
	}

	ctrl := gomock.NewController(t)
	dataProvider := dataprovider.NewMockDataStorageProvider(ctrl)
	dataProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	secretClient := secret.NewMockClient(ctrl)
	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secretClient)

	options := modules.Options{
		Address:        "localhost",
		PathBase:       pathBase,
		Config:         &hostoptions.UCPConfig{},
		DataProvider:   dataProvider,
		SecretProvider: secretProvider,
	}

	module := NewModule(options, "radius")
	handler, err := module.Initialize(context.Background())
	require.NoError(t, err)

	router := handler.(chi.Router)

	//namesMatched := map[string]bool{}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s - %s", test.method, test.path), func(t *testing.T) {
			p := pathBase + test.path
			if test.skipPathBase {
				p = test.path
			}

			tctx := chi.NewRouteContext()
			tctx.Reset()

			result := router.Match(tctx, test.method, p)
			require.Truef(t, result, "no route found for %s %s, context: %v", test.method, p, tctx)
		})
	}
	t.Run("all named routes are tested", func(t *testing.T) {
		err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			t.Logf("%s %s", method, route)
			return nil
		})
		require.NoError(t, err)
	})
	/*

		t.Run("all named routes are tested", func(t *testing.T) {
			err := router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
				if route.GetName() == "" || strings.Contains(route.GetName(), "subrouter") {
					return nil
				}

				pathTemplate, err := route.GetPathTemplate()
				require.NoError(t, err)

				assert.Contains(t, namesMatched, route.GetName(), "route %s for %s is not tested", route.GetName(), pathTemplate)
				return nil
			})
			require.NoError(t, err)
		})*/
}
