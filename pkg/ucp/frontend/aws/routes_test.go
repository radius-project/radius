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
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/secret"
	secretprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
)

const pathBase = "/some-path-base"

func Test_Routes(t *testing.T) {
	tests := []rpctest.HandlerTestSpec{
		{
			Name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationList}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/providers/System.AWS/credentials",
		}, {
			Name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationGet}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			Name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationPut}.String(),
			Method: http.MethodPut,
			Path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			Name:   v1.OperationType{Type: v20220901privatepreview.AWSCredentialType, Method: v1.OperationDelete}.String(),
			Method: http.MethodDelete,
			Path:   "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationList}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGet}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPut}.String(),
			Method: http.MethodPut,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDelete}.String(),
			Method: http.MethodDelete,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetImperative}.String(),
			Method: http.MethodPost,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:get",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPutImperative}.String(),
			Method: http.MethodPost,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:put",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDeleteImperative}.String(),
			Method: http.MethodPost,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:delete",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationResult}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationResults/00000000-0000-0000-0000-000000000000",
		}, {
			Name:   v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetOperationStatuses}.String(),
			Method: http.MethodGet,
			Path:   "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationStatuses/00000000-0000-0000-0000-000000000000",
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

	rpctest.AssertRouters(t, tests, pathBase, "", func(ctx context.Context) (chi.Router, error) {
		module := NewModule(options)
		router, err := module.Initialize(ctx)
		return router.(chi.Router), err
	})
}
