// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package aws

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/project-radius/radius/test/testutil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
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

	tests := []struct {
		name     string
		filename string
		url      string
		expected armrpc_rest.Response
		fn       func(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient)
		err      error
	}{
		{
			name:     "test_credential_creation",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			expected: getAwsResponse(),
			fn:       setupCredentialSuccessMocks,
			err:      nil,
		},
		{
			name:     "test_invalid_version_credential_resource",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2020-09-01-privatepreview",
			expected: armrpc_rest.NewBadRequestResponse(v1.ErrUnsupportedAPIVersion.Error()),
			fn:       setupEmptyMocks,
			err:      nil,
		},
		{
			name:     "test_invalid_credential_request",
			filename: "invalid-request-aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			expected: getInvalidRequestResponse(),
			fn:       setupEmptyMocks,
			err:      nil,
		},
		{
			name:     "test_credential_created",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			expected: getAwsResponse(),
			fn:       setupCredentialNotFoundMocks,
			err:      nil,
		},
		{
			name:     "test_credential_notFoundError",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupCredentialNotFoundErrorMocks,
			err:      errors.New("Error"),
		},
		{
			name:     "test_credential_get_failure",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupCredentialGetFailMocks,
			err:      errors.New("Failed Get"),
		},
		{
			name:     "test_credential_secret_save_failure",
			filename: "aws-credential.json",
			url:      "/planes/aws/awscloud/providers/System.AWS/credentials/default?api-version=2022-09-01-privatepreview",
			fn:       setupCredentialSecretSaveFailMocks,
			err:      errors.New("Secret Save Failure"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := testutil.ReadFixture(tt.filename)
			tt.fn(*mockStorageClient, *mockSecretClient)
			request, err := http.NewRequest(http.MethodPut, tt.url, bytes.NewBuffer(body))
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

func getAwsResponse() armrpc_rest.Response {
	return armrpc_rest.NewOKResponse(&v20220901privatepreview.AWSCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/aws/awscloud/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &v20220901privatepreview.AWSAccessKeyCredentialProperties{
			AccessKeyID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:        to.Ptr("AccessKey"),
			Storage: &v20220901privatepreview.InternalCredentialStorageProperties{
				Kind:       to.Ptr(string(v20220901privatepreview.CredentialStorageKindInternal)),
				SecretName: to.Ptr("aws-awscloud-default"),
			},
		},
	})
}

func getInvalidRequestResponse() armrpc_rest.Response {
	err := v1.ErrModelConversion{
		PropertyName: "$.properties",
		ValidValue:   "not nil",
	}
	return armrpc_rest.NewBadRequestResponse(err.Error())
}

func setupCredentialSuccessMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockSecretClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func setupEmptyMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
}

func setupCredentialNotFoundMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return nil, &store.ErrNotFound{}
		}).Times(1)
	mockSecretClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStorageClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func setupCredentialNotFoundErrorMocks(mockStorageClient store.MockStorageClient, mockSecretClient secret.MockClient) {
	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return nil, errors.New("Error")
		}).Times(1)
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
