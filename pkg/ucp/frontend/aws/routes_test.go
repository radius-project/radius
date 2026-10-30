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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

const pathBase = "/some-path-base"

func Test_Routes(t *testing.T) {
	tests := []rpctest.HandlerTestSpec{
		{
			OperationType: v1.OperationType{Type: datamodel.AWSPlaneResourceType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/aws",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AWSPlaneResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/aws/someName",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AWSPlaneResourceType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/aws/someName",
		}, {
			OperationType: v1.OperationType{Type: datamodel.AWSPlaneResourceType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/aws/someName",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AWSCredentialType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/providers/System.AWS/credentials",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AWSCredentialType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AWSCredentialType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			OperationType: v1.OperationType{Type: v20231001preview.AWSCredentialType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/aws/aws/providers/System.AWS/credentials/default",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/some-stream",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationGetImperative},
			Method:        http.MethodPost,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:get",
		}, {
			OperationType: v1.OperationType{Type: OperationResultsResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationResults/00000000-0000-0000-0000-000000000000",
		}, {
			OperationType: v1.OperationType{Type: OperationStatusResourceType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/locations/global/operationStatuses/00000000-0000-0000-0000-000000000000",
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

func Test_InitializeClients(t *testing.T) {
	t.Run("creates the clients that are not set", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-west-2")
		options := &ucp.Options{Config: &ucp.Config{}}
		require.NoError(t, InitializeClients(t.Context(), options))
		require.NotNil(t, options.AWSClients.CloudControl)
		require.NotNil(t, options.AWSClients.CloudFormation)
	})

	t.Run("keeps clients that are set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cc := ucp_aws.NewMockAWSCloudControlClient(ctrl)
		cf := ucp_aws.NewMockAWSCloudFormationClient(ctrl)
		options := &ucp.Options{Config: &ucp.Config{}, AWSClients: ucp_aws.Clients{CloudControl: cc, CloudFormation: cf}}
		require.NoError(t, InitializeClients(t.Context(), options))
		require.Same(t, cc, options.AWSClients.CloudControl)
		require.Same(t, cf, options.AWSClients.CloudFormation)
	})
	t.Run("uses the UCP credential API", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-west-2")
		connection, err := sdk.NewDirectConnection("http://localhost:9443")
		require.NoError(t, err)
		options := &ucp.Options{
			Config:         &ucp.Config{Identity: ucp.IdentityConfig{AuthMethod: ucp.AuthUCPCredential}},
			SecretProvider: secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{}),
			UCP:            connection,
		}
		require.NoError(t, InitializeClients(t.Context(), options))
		require.NotNil(t, options.AWSClients.CloudControl)
	})
}
