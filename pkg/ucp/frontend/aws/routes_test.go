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

package aws

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

const pathBase = "/some-path-base"

func Test_Routes(t *testing.T) {
	tests := []struct {
		method       string
		path         string
		name         string
		skipPathBase bool
	}{
		{
			name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationList}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/providers/System.AWS/credentials",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationGet}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationPut}.String(),
			method: http.MethodPut,
			path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationDelete}.String(),
			method: http.MethodDelete,
			path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationList}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGet}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPut}.String(),
			method: http.MethodPut,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDelete}.String(),
			method: http.MethodDelete,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetImperative}.String(),
			method: http.MethodPost,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:get",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPutImperative}.String(),
			method: http.MethodPost,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:put",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDeleteImperative}.String(),
			method: http.MethodPost,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:delete",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationResult}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationResults/00000000-0000-0000-0000-000000000000",
		}, {
			name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationStatuses}.String(),
			method: http.MethodGet,
			path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationStatuses/00000000-0000-0000-0000-000000000000",
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

	module := NewModule(options)
	handler, err := module.Initialize(context.Background())
	require.NoError(t, err)

	router := chi.NewRouter()

	router.Mount(pathBase+prefixPath, handler)

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_%s", test.method, test.path), func(t *testing.T) {
			p := pathBase + test.path
			if test.skipPathBase {
				p = test.path
			}

			tctx := chi.NewRouteContext()
			tctx.Reset()

			result := router.Match(tctx, test.method, p)
			require.Truef(t, result, "no route found for %s %s", test.method, p)
		})
	}

	t.Run("all named routes are tested", func(t *testing.T) {
		err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			t.Logf("%s %s", method, route)
			return nil
		})
		require.NoError(t, err)
	})
}
