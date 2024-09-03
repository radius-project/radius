package configloader

import (
	"testing"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_populateSecretData(t *testing.T) {
	secretStoreDataTypeGeneric := v20231001preview.SecretStoreDataTypeGeneric
	tests := []struct {
		name            string
		secretKeys      []string
		secrets         *v20231001preview.SecretStoresClientListSecretsResponse
		secretStoreID   string
		expectedSecrets recipes.SecretData
		expectError     bool
		expectedErrMsg  string
	}{
		{
			name:       "success - data for input secretKey1 returned",
			secretKeys: []string{"secretKey1"},
			secrets: &v20231001preview.SecretStoresClientListSecretsResponse{
				SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
					Type: &secretStoreDataTypeGeneric,
					Data: map[string]*v20231001preview.SecretValueProperties{
						"secretKey1": {
							Value: to.Ptr("secretValue1"),
						},
						"secretKey2": {
							Value: to.Ptr("secretValue2"),
						},
					}},
			},
			secretStoreID: "testSecretStore",
			expectedSecrets: recipes.SecretData{
				Type: "generic",
				Data: map[string]string{"secretKey1": "secretValue1"},
			},
			expectError: false,
		},
		{
			name:       "success - data for all keys returned with nil secretKeys input",
			secretKeys: nil,
			secrets: &v20231001preview.SecretStoresClientListSecretsResponse{
				SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
					Type: &secretStoreDataTypeGeneric,
					Data: map[string]*v20231001preview.SecretValueProperties{
						"secretKey1": {
							Value: to.Ptr("secretValue1"),
						},
						"secretKey2": {
							Value: to.Ptr("secretValue2"),
						},
					}},
			},
			secretStoreID: "testSecretStore",
			expectedSecrets: recipes.SecretData{
				Type: "generic",
				Data: map[string]string{
					"secretKey1": "secretValue1",
					"secretKey2": "secretValue2",
				},
			},
			expectError: false,
		},
		{
			name:       "success - returned with nil secretKeys input when no secret data exist",
			secretKeys: nil,
			secrets: &v20231001preview.SecretStoresClientListSecretsResponse{
				SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
					Type: &secretStoreDataTypeGeneric,
					Data: nil},
			},
			secretStoreID: "testSecretStore",
			expectedSecrets: recipes.SecretData{
				Type: "generic",
				Data: map[string]string{},
			},
			expectError: false,
		},
		{
			name:            "fail - nil secrets input",
			secretKeys:      []string{"secretKey1"},
			secrets:         nil,
			secretStoreID:   "testSecretStore",
			expectedSecrets: recipes.SecretData{},
			expectError:     true,
			expectedErrMsg:  "secrets not found for secret store ID 'testSecretStore'",
		},
		{
			name:       "fail - missing secret key",
			secretKeys: []string{"missingKey"},
			secrets: &v20231001preview.SecretStoresClientListSecretsResponse{
				SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
					Type: &secretStoreDataTypeGeneric,
				},
			},
			secretStoreID:   "testSecretStore",
			expectedSecrets: recipes.SecretData{},
			expectError:     true,
			expectedErrMsg:  "a secret key was not found in secret store 'testSecretStore'",
		},
		{
			name:       "fail - missing secret type",
			secretKeys: []string{"secretKey1"},
			secrets: &v20231001preview.SecretStoresClientListSecretsResponse{
				SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
					Data: map[string]*v20231001preview.SecretValueProperties{
						"secretKey1": {
							Value: to.Ptr("secretValue1"),
						},
						"secretKey2": {
							Value: to.Ptr("secretValue2"),
						},
					}},
			},
			secretStoreID:   "testSecretStore",
			expectedSecrets: recipes.SecretData{},
			expectError:     true,
			expectedErrMsg:  "secret store type is not set for secret store ID 'testSecretStore'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretData, err := populateSecretData(tt.secretStoreID, tt.secretKeys, tt.secrets)
			if tt.expectError {
				require.EqualError(t, err, tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedSecrets, secretData)
			}
		})
	}
}
