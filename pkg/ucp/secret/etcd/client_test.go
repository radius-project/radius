// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/stretchr/testify/require"
)

const (
	testSecretName = "azure_azurecloud_default"
)

func Test_SaveSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	ctx := context.Background()
	mockETCDClient := NewMockETCDV3Client(mctrl)
	client := Client{
		ETCDClient: mockETCDClient,
	}
	testSecret, err := json.Marshal("test_secret")
	require.NoError(t, err)
	tests := []struct {
		testName     string
		secretClient *MockETCDV3Client
		secretName   string
		secret       []byte
		err          error
	}{
		{"save-secret-success", mockETCDClient, testSecretName, testSecret, nil},
		{"save-secret-fail", mockETCDClient, testSecretName, testSecret, errors.New("failed to save secret")},
		{"save-secret-empty-name", mockETCDClient, "", testSecret, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
		{"save-secret-empty-secret", mockETCDClient, testSecretName, nil, &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.secretName != "" && tt.secret != nil {
				mockETCDClient.EXPECT().
					Save(context.Background(), gomock.Any(), gomock.Any()).
					Return(tt.err).Times(1)
			}
			err = client.Save(ctx, tt.secretName, tt.secret)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, err, tt.err)
			}
		})
	}
}

func Test_DeleteSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	ctx := context.Background()
	mockETCDClient := NewMockETCDV3Client(mctrl)
	client := Client{
		ETCDClient: mockETCDClient,
	}
	tests := []struct {
		testName     string
		secretClient *MockETCDV3Client
		secretName   string
		err          error
	}{
		{"delete-secret-success", mockETCDClient, testSecretName, nil},
		{"delete-secret-fail", mockETCDClient, testSecretName, errors.New("unable to delete secrets")},
		{"delete-secret-empty-name", mockETCDClient, testSecretName, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.secretName != "" {
				mockETCDClient.EXPECT().
					Delete(context.Background(), gomock.Any()).
					Return(tt.err).Times(1)
			}
			err := client.Delete(ctx, tt.secretName)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, err, tt.err)
			}
		})
	}
}

func Test_GetSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	ctx := context.Background()
	mockETCDClient := NewMockETCDV3Client(mctrl)
	client := Client{
		ETCDClient: mockETCDClient,
	}
	getResponse := []byte("test-secret")
	tests := []struct {
		testName       string
		secretClient   *MockETCDV3Client
		secretName     string
		secretResponse []byte
		err            error
	}{
		{"get-secret-success", mockETCDClient, testSecretName, getResponse, nil},
		{"get-secret-fail", mockETCDClient, testSecretName, nil, errors.New("unable to delete secrets")},
		{"get-secret-empty-name", mockETCDClient, testSecretName, nil, &secret.ErrNotFound{}},
		{"get-secret-empty-name", mockETCDClient, testSecretName, nil, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.secretName != "" {
				mockETCDClient.EXPECT().
					Get(context.Background(), tt.secretName).
					Return(tt.secretResponse, tt.err).Times(1)
			}
			response, err := client.Get(ctx, tt.secretName)
			if tt.err == nil {
				require.NoError(t, err)
				require.Equal(t, string(response), string(getResponse))
			} else {
				require.Equal(t, err, tt.err)
			}
		})
	}
}
