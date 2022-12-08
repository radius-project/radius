// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package credentials

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_Credential(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockSecretClient := secret.NewMockClient(mockCtrl)

	credentialCtrl, err := NewCreateOrUpdateCredential(ctrl.Options{
		DB:           mockStorageClient,
		SecretClient: mockSecretClient,
	})
	require.NoError(t, err)

	body := []byte(`{
		"id": "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
		"name": "default",
		"type": "System.Azure/credentials",
		"location": "west-us-2",
		"properties": {
			"tenantId": "00000000-0000-0000-0000-000000000000",
			"clientId": "00000000-0000-0000-0000-000000000000",
			"secret":   "secret",
			"kind":     "azure.com.serviceprincipal",
			"storage": {
				"kind": "Internal"
			}
		}
	}`)
	url := "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview"
	versionedCredential := v20220901privatepreview.CredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/azure/azurecloud/providers/System.Azure/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.Azure/credentials"),
		Properties: &v20220901privatepreview.AzureServicePrincipalProperties{
			ClientID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			TenantID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:     to.Ptr("azure.com.serviceprincipal"),
			Storage: &v20220901privatepreview.InternalCredentialStorageProperties{
				Kind:       to.Ptr(v20220901privatepreview.CredentialStorageKindInternal),
				SecretName: to.Ptr("azure_azurecloud_default"),
			},
		},
	}

	tests := []struct {
		name string
		fn   func(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient)
		err  error
	}{
		{
			name: "test_credential_creation",
			fn:   setupCredentialSuccessMocks,
			err:  nil,
		},
		{
			name: "test_credential_notFound",
			fn:   setupCredentialNotFoundMocks,
			err:  nil,
		},
		{
			name: "test_credential_get_failure",
			fn:   setupCredentialGetFailMocks,
			err:  errors.New("Failed Get"),
		},
		{
			name: "test_credential_secret_save_failure",
			fn:   setupCredentialSecretSaveFailMocks,
			err:  errors.New("Secret Save Failure"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(*mockStorageClient, *mockSecretClient)
			request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
			require.NoError(t, err)
			response, err := credentialCtrl.Run(ctx, nil, request)
			if err != nil {
				require.Equal(t, err, tt.err)
			} else {
				require.NoError(t, err)
				expectedResponse := armrpc_rest.NewOKResponse(&versionedCredential)
				assert.DeepEqual(t, expectedResponse, response)
			}
		})
	}
}

func setupCredentialSuccessMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func setupCredentialNotFoundMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return nil, &store.ErrNotFound{}
		}).Times(1)
	mockSecretClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func setupCredentialGetFailMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return nil, errors.New("Failed Get")
		}).Times(1)
}

func setupCredentialSecretSaveFailMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("Secret Save Failure")).Times(1)
}
