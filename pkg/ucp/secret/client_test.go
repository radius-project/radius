// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secret

import (
	"context"
	"errors"
	"testing"

	gomock "github.com/golang/mock/gomock"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/stretchr/testify/require"
)

func Test_SaveAzureSecretSuccessfully(t *testing.T) {
	azureSecret, err := GetTestAzureSecret()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	mockClient.EXPECT().
		Save(context.Background(), gomock.Any(), TestSecretId).
		Return(nil).Times(1)
	err = SaveSecret(context.Background(), azureSecret, TestSecretId, mockClient)
	require.NoError(t, err)
}

func Test_SaveAzureSecretFail(t *testing.T) {
	azureSecret, err := GetTestAzureSecret()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	saveError := errors.New("Failed to Save Secret")
	mockClient.EXPECT().
		Save(context.Background(), gomock.Any(), TestSecretId).
		Return(saveError).Times(1)
	err = SaveSecret(context.Background(), azureSecret, TestSecretId, mockClient)
	require.Equal(t, err, saveError)
}

func Test_GetAzureSecretSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	testSecretResponse, err := GetTestAzureSecretResponse()
	require.NoError(t, err)
	mockClient.EXPECT().
		Get(context.Background(), TestSecretId).
		Return(testSecretResponse, nil).Times(1)
	azureSecret, err := GetSecret[ucp.AzureServicePrincipalProperties](context.Background(), TestSecretId, mockClient)
	require.NoError(t, err)
	require.Equal(t, *azureSecret.ClientID, "clientId")
}

func Test_GetAzureSecretFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	getError := errors.New("Failed to Save Secret")
	mockClient.EXPECT().
		Get(context.Background(), TestSecretId).
		Return(nil, getError).Times(1)
	// We don't care about response when testing for error
	_, err := GetSecret[ucp.AzureServicePrincipalProperties](context.Background(), TestSecretId, mockClient)
	require.Equal(t, err, getError)
}
