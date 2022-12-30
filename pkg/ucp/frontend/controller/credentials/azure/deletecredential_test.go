// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package azure

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_Credential_Delete(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockSecretClient := secret.NewMockClient(mockCtrl)

	credentialCtrl, err := NewDeleteCredential(ctrl.Options{
		DB:           mockStorageClient,
		SecretClient: mockSecretClient,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		url      string
		fn       func(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient)
		expected armrpc_rest.Response
		err      error
	}{
		{
			name:     "test_credential_deletion",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupCredentialDeleteSuccessMocks,
			expected: rest.NewOKResponse(nil),
			err:      nil,
		},
		{
			name:     "test_non_existent_credential_deletion",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupNonExistentCredentialDeleteMocks,
			expected: armrpc_rest.NewNoContentResponse(),
			err:      nil,
		},
		{
			name:     "test_failed_credential_existence_check",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupCredentialExistenceCheckFailureMocks,
			expected: nil,
			err:      errors.New("test_failure"),
		},
		{
			name:     "test_non_existent_secret_deletion",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupNonExistentSecretDeleteMocks,
			expected: armrpc_rest.NewNoContentResponse(),
			err:      nil,
		},
		{
			name:     "test_secret_deletion_failure",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupSecretDeleteFailureMocks,
			expected: nil,
			err:      errors.New("Failed secret deletion"),
		},
		{
			name:     "test_non_existing_credential_deletion_from_storage",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupNonExistingCredentialDeleteFromStorageMocks,
			expected: armrpc_rest.NewNoContentResponse(),
			err:      nil,
		},
		{
			name:     "test_failed_credential_deletion_from_storage",
			url:      "/planes/azure/azurecloud/providers/System.Azure/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupFailedCredentialDeleteFromStorageMocks,
			expected: nil,
			err:      errors.New("Failed Storage Deletion"),
		},
		{
			name: "test_invalid_url_credential_delete",
			url:  "/planes/azure/azurecloud/providers/System.Azure//default?api-version=2022-09-01-privatepreview",
			fn:   setupEmptyMocks,
			expected: armrpc_rest.NewBadRequestResponse(
				fmt.Errorf("'%s' is not a valid resource id",
					"azure/azurecloud/providers/System.Azure//default").Error()),
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(*mockStorageClient, *mockSecretClient)
			request, err := http.NewRequest(http.MethodDelete, tt.url, nil)
			require.NoError(t, err)
			response, err := credentialCtrl.Run(ctx, nil, request)
			if tt.err != nil {
				require.Equal(t, err, tt.err)
			} else {
				require.NoError(t, err)
				assert.DeepEqual(t, tt.expected, response)
			}
		})
	}
}

func setupCredentialDeleteSuccessMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func setupNonExistentCredentialDeleteMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &store.ErrNotFound{}).Times(1)
}

func setupCredentialExistenceCheckFailureMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("test_failure")).Times(1)
}

func setupNonExistentSecretDeleteMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(&secret.ErrNotFound{}).Times(1)
}

func setupSecretDeleteFailureMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(errors.New("Failed secret deletion")).Times(1)
}

func setupNonExistingCredentialDeleteFromStorageMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.ErrNotFound{}).Times(1)
}

func setupFailedCredentialDeleteFromStorageMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("Failed Storage Deletion")).Times(1)
}
