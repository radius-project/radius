// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	SecretName = "test_secret_name"
)

func Test_Save(t *testing.T) {
	k8sFakeClient := Client{
		K8sClient: fake.NewClientBuilder().Build(),
	}
	runTests(t, &k8sFakeClient)
}

func runTests(t *testing.T, k8sClient *Client) {
	ctx := context.Background()
	secretValue, err := json.Marshal("test_secret_value")
	require.NoError(t, err)

	tests := []struct {
		testName   string
		secretName string
		secret     []byte
		save       bool
		saveAgain  bool
		get        bool
		delete     bool
		err        error
	}{
		{"save-new-secret-success", SecretName, secretValue, true, false, true, true, nil},
		{"update-existing-secret-success", SecretName, secretValue, true, true, true, true, nil},
		{"get-non-existent-secret", SecretName, secretValue, false, false, true, false, &secret.ErrNotFound{}},
		{"delete-non-existent-secret", SecretName, secretValue, false, false, false, true, &secret.ErrNotFound{}},
		{"save-with-invalid-name", "", secretValue, true, false, false, false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
		{"save-with-empty-secret", SecretName, nil, true, false, false, false, &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.save {
				err := k8sClient.Save(ctx, tt.secretName, tt.secret)
				require.Equal(t, err, tt.err)
			}
			if tt.saveAgain {
				err := k8sClient.Save(ctx, tt.secretName, tt.secret)
				require.NoError(t, err)
			}
			if tt.get {
				res, err := k8sClient.Get(ctx, tt.secretName)
				require.Equal(t, err, tt.err)
				if tt.err == nil {
					require.Equal(t, res, secretValue)
				}
			}
			if tt.delete {
				err := k8sClient.Delete(ctx, tt.secretName)
				require.Equal(t, err, tt.err)
			}
		})
	}
}
