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

package testrp

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/armrpc/servicecontext"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// SyncResource creates an HTTP handler that can be used to test synchronous resource lifecycle operations.
func SyncResource(t *testing.T, ts *testserver.TestServer, rootScope string) func(w http.ResponseWriter, r *http.Request) {
	rootScope = strings.ToLower(rootScope)

	ctx := testcontext.New(t)
	r := chi.NewRouter()
	r.Use(servicecontext.ARMRequestCtx("", v1.LocationGlobal), middleware.LowercaseURLPath)

	ctrlOpts := frontend_ctrl.Options{
		DataProvider: ts.Clients.StorageProvider,
	}

	err := server.ConfigureDefaultHandlers(ctx, r, rootScope, false, "System.Test", nil, ctrlOpts)
	require.NoError(t, err)

	resourceType := "System.Test/testResources"
	rootScopeRouter := server.NewSubrouter(r, rootScope)
	testResourceCollectionRouter := server.NewSubrouter(rootScopeRouter, "/providers/system.test/testresources")
	testResourceSingleRouter := server.NewSubrouter(rootScopeRouter, "/providers/system.test/testresources/{testResourceName}")

	resourceOptions := frontend_ctrl.ResourceOptions[TestResourceDatamodel]{
		RequestConverter:  TestResourceDataModelFromVersioned,
		ResponseConverter: TestResourceDataModelToVersioned,
	}

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: testResourceCollectionRouter,
			ResourceType: resourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt, resourceOptions)
			},
		},
	}

	for _, h := range handlerOptions {
		err := server.RegisterHandler(ctx, h, ctrlOpts)
		require.NoError(t, err)
	}

	return r.ServeHTTP
}
