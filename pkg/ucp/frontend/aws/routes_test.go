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
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
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
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationPutImperative},
			Method:        http.MethodPost,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:put",
		}, {
			OperationType: v1.OperationType{Type: OperationTypeAWSResource, Method: v1.OperationDeleteImperative},
			Method:        http.MethodPost,
			Path:          "/planes/aws/aws/accounts/0000000/regions/some-region/providers/AWS.Kinesis/Stream/:delete",
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

	options := newTestOptions(t)

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
	t.Run("fails when the AWS configuration cannot be loaded", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "radius-test-profile-that-does-not-exist")
		require.Error(t, InitializeClients(t.Context(), &ucp.Options{Config: &ucp.Config{}}))

		_, err := NewModule(newTestOptions(t)).Initialize(t.Context())
		require.Error(t, err)
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

func Test_Initialize_FingerprintKey(t *testing.T) {
	t.Run("loads the key from the configured file", func(t *testing.T) {
		raw := bytes.Repeat([]byte{1}, ucp_aws.MinFingerprintKeyLength)
		path := filepath.Join(t.TempDir(), "key")
		require.NoError(t, os.WriteFile(path, raw, 0400))
		want, err := ucp_aws.NewFingerprintKey(raw)
		require.NoError(t, err)

		options := newTestOptions(t)
		options.Config.AWS.FingerprintKeyFile = path
		module := NewModule(options)
		ctx, logs := captureLogs(t)
		_, err = module.Initialize(ctx)
		require.NoError(t, err)

		key, err := module.FingerprintKey()
		require.NoError(t, err)
		require.Equal(t, want.ID(), key.ID())

		require.Contains(t, *logs, map[string]any{"logger": "", "level": float64(0), "msg": "Loaded the AWS fingerprint key", "keyID": want.ID()})
		for _, entry := range *logs {
			require.NotContains(t, entry, "error")
		}
	})

	t.Run("starts without the key and keeps the error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")

		options := newTestOptions(t)
		options.Config.AWS.FingerprintKeyFile = path
		module := NewModule(options)
		ctx, logs := captureLogs(t)
		handler, err := module.Initialize(ctx)
		require.NoError(t, err)
		require.NotNil(t, handler)

		// The error is logged when UCP starts, before any request needs the key.
		key, err := module.FingerprintKey()
		require.Nil(t, key)
		var errorLogs []map[string]any
		for _, entry := range *logs {
			if _, ok := entry["error"]; ok {
				errorLogs = append(errorLogs, entry)
			}
		}
		require.Equal(t, []map[string]any{{
			"logger": "",
			"msg":    "The AWS fingerprint key is not available; AWS resource PUTs that need a fingerprint of a write-only value will fail",
			"error":  err.Error(),
			"secret": ucp_aws.FingerprintKeySecretName,
		}}, errorLogs)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.ErrorContains(t, err, ucp_aws.FingerprintKeySecretName)
		require.ErrorContains(t, err, path)
	})

	t.Run("defaults to the path the Helm chart mounts", func(t *testing.T) {
		if _, err := os.Stat(ucp_aws.DefaultFingerprintKeyFile); err == nil {
			t.Skip("The default key file exists on this machine.")
		}

		module := NewModule(newTestOptions(t))
		_, err := module.Initialize(t.Context())
		require.NoError(t, err)

		_, err = module.FingerprintKey()
		require.ErrorContains(t, err, ucp_aws.DefaultFingerprintKeyFile)
	})
}

// captureLogs returns a context whose logger records the structured fields of each log entry.
func captureLogs(t *testing.T) (context.Context, *[]map[string]any) {
	t.Helper()
	logs := &[]map[string]any{}
	logger := funcr.NewJSON(func(obj string) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(obj), &entry))
		*logs = append(*logs, entry)
	}, funcr.Options{})
	return logr.NewContext(t.Context(), logger), logs
}

func newTestOptions(t *testing.T) *ucp.Options {
	t.Helper()
	ctrl := gomock.NewController(t)

	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secret.NewMockClient(ctrl))

	return &ucp.Options{
		Config: &ucp.Config{
			Server: hostoptions.ServerOptions{
				Host:     "localhost",
				Port:     8080,
				PathBase: pathBase,
			},
		},
		DatabaseProvider: databaseprovider.FromMemory(),
		SecretProvider:   secretProvider,
		StatusManager:    statusmanager.NewMockStatusManager(ctrl),
	}
}
