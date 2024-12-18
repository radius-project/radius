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
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/hostoptions"
	"go.uber.org/mock/gomock"
)

const pathBase = "/some-path-base"

func Test_Routes(t *testing.T) {
	tests := []rpctest.HandlerTestSpec{
		// Radius plane
		{
			OperationType: v1.OperationType{Type: datamodel.RadiusPlaneResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.RadiusPlaneResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.RadiusPlaneResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/someName",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.RadiusPlaneResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/someName",
		},

		// Resource types
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceProviderResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceProviderResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceProviderResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceProviderResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.LocationResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/locations",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.LocationResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/locations/east",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.LocationResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/locations/east",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.LocationResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/locations/east",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceTypeResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceTypeResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceTypeResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.ResourceTypeResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.APIVersionResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources/apiversions",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.APIVersionResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources/apiversions/2025-01-01",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.APIVersionResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources/apiversions/2025-01-01",
		},
		{
			OperationType: v1.OperationType{Type: datamodel.APIVersionResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/someName/providers/System.Resources/resourceproviders/Applications.Test/resourcetypes/testResources/apiversions/2025-01-01",
		},

		// Resource groups
		{
			OperationType: v1.OperationType{Type: v20231001preview.ResourceGroupType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/radius/local/resourcegroups",
		},
		{
			OperationType: v1.OperationType{Type: v20231001preview.ResourceGroupType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/radius/local/resourcegroups/test-rg",
		},
		{
			OperationType: v1.OperationType{Type: v20231001preview.ResourceGroupType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/radius/local/resourcegroups/test-rg",
		},
		{
			OperationType: v1.OperationType{Type: v20231001preview.ResourceGroupType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/radius/local/resourcegroups/test-rg",
		},
		{
			OperationType:               v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			Method:                      http.MethodGet,
			Path:                        "/planes/radius/local/providers/applications.core/applications/test-app",
			SkipOperationTypeValidation: true,
		},

		// Proxy
		{
			OperationType:               v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			Method:                      http.MethodGet,
			Path:                        "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/test-app",
			SkipOperationTypeValidation: true,
		}, {
			OperationType:               v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			Method:                      http.MethodPut,
			Path:                        "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/test-app",
			SkipOperationTypeValidation: true,
		},
	}

	ctrl := gomock.NewController(t)
	databaseProvider := databaseprovider.FromMemory()

	secretClient := secret.NewMockClient(ctrl)
	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secretClient)

	options := modules.Options{
		Address:          "localhost",
		PathBase:         pathBase,
		Config:           &hostoptions.UCPConfig{},
		DatabaseProvider: databaseProvider,
		SecretProvider:   secretProvider,
	}

	rpctest.AssertRouters(t, tests, pathBase, "", func(ctx context.Context) (chi.Router, error) {
		module := NewModule(options)
		handler, err := module.Initialize(ctx)
		if err != nil {
			return nil, err
		}

		return handler.(chi.Router), nil
	})
}
