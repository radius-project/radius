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

package secret

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	genfake "github.com/radius-project/radius/pkg/cli/clients_new/generated/fake"
	"github.com/stretchr/testify/require"
)

const testOwnerID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/kafkas/kafka"

func Test_ManagedSecretName(t *testing.T) {
	require.Equal(t, "kafka-secrets", ManagedSecretName("kafka"))
}

func Test_Materialize(t *testing.T) {
	var capturedName string
	var captured generated.GenericResource

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginCreateOrUpdate: func(ctx context.Context, resourceName string, params generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
				capturedName = resourceName
				captured = params
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientCreateOrUpdateResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	result, err := m.Materialize(context.Background(), Request{
		OwnerResourceID: testOwnerID,
		EnvironmentID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env",
		ApplicationID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app",
		Data:            map[string]string{"connectionString": "abc"},
	})
	require.NoError(t, err)

	require.Equal(t, "kafka-secrets", capturedName)
	require.Equal(t, "kafka-secrets", result.Name)
	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Security/secrets/kafka-secrets", result.ID)

	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env", captured.Properties["environment"])
	require.Equal(t, "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app", captured.Properties["application"])

	data, ok := captured.Properties["data"].(map[string]any)
	require.True(t, ok)
	connectionString, ok := data["connectionString"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "abc", connectionString["value"])
}

func Test_Materialize_OmitsEmptyApplication(t *testing.T) {
	var captured generated.GenericResource

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginCreateOrUpdate: func(ctx context.Context, resourceName string, params generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
				captured = params
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientCreateOrUpdateResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	_, err := m.Materialize(context.Background(), Request{
		OwnerResourceID: testOwnerID,
		EnvironmentID:   "env",
		Data:            map[string]string{"connectionString": "abc"},
	})
	require.NoError(t, err)

	_, hasApp := captured.Properties["application"]
	require.False(t, hasApp)
}

func Test_Materialize_InvalidOwnerID(t *testing.T) {
	m := NewMaterializer(&arm.ClientOptions{})
	_, err := m.Materialize(context.Background(), Request{OwnerResourceID: "not-an-id", Data: map[string]string{"k": "v"}})
	require.Error(t, err)
}

func Test_Delete(t *testing.T) {
	var capturedName string

	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginDelete: func(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientDeleteResponse], errResp azfake.ErrorResponder) {
				capturedName = resourceName
				resp.SetTerminalResponse(http.StatusOK, generated.GenericResourcesClientDeleteResponse{}, nil)
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	require.NoError(t, m.Delete(context.Background(), testOwnerID))
	require.Equal(t, "kafka-secrets", capturedName)
}

func Test_Delete_NotFoundIsIgnored(t *testing.T) {
	transport := genfake.NewServerFactoryTransport(&genfake.ServerFactory{
		GenericResourcesServer: genfake.GenericResourcesServer{
			BeginDelete: func(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (resp azfake.PollerResponder[generated.GenericResourcesClientDeleteResponse], errResp azfake.ErrorResponder) {
				errResp.SetResponseError(http.StatusNotFound, "NotFound")
				return
			},
		},
	})

	m := NewMaterializer(&arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: transport}})

	require.NoError(t, m.Delete(context.Background(), testOwnerID))
}
