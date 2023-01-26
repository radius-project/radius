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
	testSecretName = "azure-azurecloud-default"
)

func Test_SaveSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSecretClient := NewMockClient(mctrl)
	ctx := context.Background()
	azureSecret, err := newTestAzureSecret()
	require.NoError(t, err)
	saveError := errors.New("Failed to Save Secret")
	tests := []struct {
		testName     string
		secretClient *MockClient
		secret       testSecretObject
		isSuccess    bool
	}{
		{"save-secret-success", mockSecretClient, azureSecret, true},
		{"save-secret-fail", mockSecretClient, azureSecret, false},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.isSuccess {
				mockSecretClient.EXPECT().
					Save(context.Background(), testSecretName, gomock.Any()).
					Return(nil).Times(1)
			} else {
				mockSecretClient.EXPECT().
					Save(context.Background(), testSecretName, gomock.Any()).
					Return(saveError).Times(1)
			}
			err := SaveSecret(ctx, tt.secretClient, testSecretName, tt.secret)
			if tt.isSuccess {
				require.NoError(t, err)
			} else {
				require.Equal(t, saveError, err)
			}
		})
	}
}

func Test_GetSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSecretClient := NewMockClient(mctrl)
	ctx := context.Background()

	testSecretResponse, err := newTestAzureSecretResponse()
	getError := errors.New("Failed to Save Secret")

	tests := []struct {
		testName     string
		secretClient *MockClient
		isSuccess    bool
	}{
		{"get-secret-success", mockSecretClient, true},
		{"get-secret-fail", mockSecretClient, false},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.isSuccess {
				mockSecretClient.EXPECT().
					Get(context.Background(), testSecretName).
					Return(testSecretResponse, nil).Times(1)
			} else {
				mockSecretClient.EXPECT().
					Get(context.Background(), testSecretName).
					Return(nil, getError).Times(1)
			}
			secretResponse, err := GetSecret[testSecretObject](ctx, tt.secretClient, testSecretName)
			if tt.isSuccess {
				require.NoError(t, err)
				require.Equal(t, secretResponse.ClientID, "clientId")
			} else {
				require.NotNil(t, secretResponse)
				require.Equal(t, err, getError)
			}
		})
	}

	require.NoError(t, err)
}

type testSecretObject struct {
	ClientID string `json:"clientId,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Secret   string `json:"secret,omitempty"`
	TenantID string `json:"tenantId,omitempty"`
}

func newTestAzureSecret() (testSecretObject, error) {
	return testSecretObject{
		Kind:     "azure",
		ClientID: "clientId",
		Secret:   "secret",
		TenantID: "tenantId",
	}, nil
}

func newTestAzureSecretResponse() ([]byte, error) {
	secret, err := newTestAzureSecret()
	if err != nil {
		return nil, err
	}
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	return secretBytes, nil
}
