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

package azure

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

const pathBase = "/some-path-base"

func Test_Routes(t *testing.T) {
	tests := []rpctest.HandlerTestSpec{
		{
			OperationType: v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/azure",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/azure/someName",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/azure/someName",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/azure/someName",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AzureCredentialType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/azure/azurecloud/providers/System.Azure/credentials",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AzureCredentialType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AzureCredentialType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AzureCredentialType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
		}, {
			OperationType:               v1.OperationType{Type: OperationTypeUCPAzureProxy, Method: v1.OperationProxy},
			Method:                      http.MethodGet,
			Path:                        "/planes/azure/azurecloud/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/some-group/providers/Microsoft.Storage/storageAccounts/test-account",
			SkipOperationTypeValidation: true,
		}, {
			OperationType:               v1.OperationType{Type: OperationTypeUCPAzureProxy, Method: v1.OperationProxy},
			Method:                      http.MethodPut,
			Path:                        "/planes/azure/azurecloud/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/some-group/providers/Microsoft.Storage/storageAccounts/test-account",
			SkipOperationTypeValidation: true,
		},
	}

	ctrl := gomock.NewController(t)

	secretClient := secret.NewMockClient(ctrl)
	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secretClient)

	options := &ucp.Options{
		Config: &ucp.Config{
			Server: hostoptions.ServerOptions{
				Host:     "localhost",
				Port:     8080,
				PathBase: pathBase,
			},
		},
		DatabaseProvider: databaseprovider.FromMemory(),
		SecretProvider:   secretProvider,
		StatusManager:    statusmanager.NewMockStatusManager(gomock.NewController(t)),
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
