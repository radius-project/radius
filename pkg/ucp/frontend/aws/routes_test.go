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
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

		log := strings.Join(*logs, "\n")
		require.Contains(t, log, `"msg"="Loaded the AWS fingerprint key" "keyID"="`+want.ID()+`"`)
		require.NotContains(t, log, `"error"`)
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
		var errorLogs []string
		for _, line := range *logs {
			if strings.Contains(line, `"error"=`) {
				errorLogs = append(errorLogs, line)
			}
		}
		require.Len(t, errorLogs, 1)
		require.Contains(t, errorLogs[0], `"msg"="The AWS fingerprint key is not available; AWS resource PUTs that need a fingerprint of a write-only value will fail"`)
		require.Contains(t, errorLogs[0], `"error"="failed to read the AWS fingerprint key from the Kubernetes Secret radius-aws-fingerprint-key mounted at `+path)
		require.Contains(t, errorLogs[0], `"secret"="radius-aws-fingerprint-key"`)

		key, err := module.FingerprintKey()
		require.Nil(t, key)
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

// captureLogs returns a context whose logger records each log line.
func captureLogs(t *testing.T) (context.Context, *[]string) {
	t.Helper()
	logs := &[]string{}
	logger := funcr.New(func(prefix, args string) {
		*logs = append(*logs, prefix+args)
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
