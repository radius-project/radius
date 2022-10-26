// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secret

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	TestSecretId = "azure_azurecloud_default"
)

func Test_SaveAzureSecretSuccessfully(t *testing.T) {
	azureSecret, err := NewTestAzureSecret()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	mockClient.EXPECT().
		Save(context.Background(), TestSecretId, gomock.Any(),).
		Return(nil).Times(1)
	err = SaveSecret(context.Background(), azureSecret, TestSecretId, mockClient)
	require.NoError(t, err)
}

func Test_SaveAzureSecretFail(t *testing.T) {
	azureSecret, err := NewTestAzureSecret()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	saveError := errors.New("Failed to Save Secret")
	mockClient.EXPECT().
		Save(context.Background(), TestSecretId, gomock.Any()).
		Return(saveError).Times(1)
	err = SaveSecret(context.Background(), azureSecret, TestSecretId, mockClient)
	require.Equal(t, err, saveError)
}

func Test_GetAzureSecretSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	testSecretResponse, err := NewTestAzureSecretResponse()
	require.NoError(t, err)
	mockClient.EXPECT().
		Get(context.Background(), TestSecretId).
		Return(testSecretResponse, nil).Times(1)
	azureSecret, err := GetSecret[TestSecretObject](context.Background(), TestSecretId, mockClient)
	require.NoError(t, err)
	require.Equal(t, azureSecret.ClientID, "clientId")
}

func Test_GetAzureSecretFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockClient(ctrl)
	getError := errors.New("Failed to Save Secret")
	mockClient.EXPECT().
		Get(context.Background(), TestSecretId).
		Return(nil, getError).Times(1)
	// We don't care about response when testing for error
	azureSecret, err := GetSecret[TestSecretObject](context.Background(), TestSecretId, mockClient)
	require.NotNil(t, azureSecret)
	require.Equal(t, err, getError)
}

type TestSecretObject struct {
	ClientID string `json:"clientId,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Secret   string `json:"secret,omitempty"`
	TenantID string `json:"tenantId,omitempty"`
}

func NewTestAzureSecret() (TestSecretObject, error) {
	return TestSecretObject{
		Kind:     "azure",
		ClientID: "clientId",
		Secret:   "secret",
		TenantID: "tenantId",
	}, nil
}

func NewTestAzureSecretResponse() ([]byte, error) {
	secret, err := NewTestAzureSecret()
	if err != nil {
		return nil, err
	}
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	return secretBytes, nil
}
