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

package inmemory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/secret"

	"github.com/stretchr/testify/require"
)

const (
	secretName = "test-secret-name"
)

func Test_Save(t *testing.T) {
	ctx := context.Background()

	secretValue, err := json.Marshal("test_secret_value")
	require.NoError(t, err)

	updatedSecretValue, err := json.Marshal("updated_secret_value")
	require.NoError(t, err)

	tests := []struct {
		testName    string
		secretName  string
		secretValue []byte
		update      bool
		err         error
	}{
		{"save-new-secret", secretName, secretValue, false, nil},
		{"update-secret", secretName, secretValue, true, nil},
		{"save-with-invalid-name", "", secretValue, false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
		{"save-with-empty-secret", secretName, nil, false, &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			client := Client{}

			err := client.Save(ctx, tt.secretName, tt.secretValue)
			require.Equal(t, err, tt.err)

			if tt.update {
				err := client.Save(ctx, tt.secretName, updatedSecretValue)
				require.Equal(t, err, tt.err)
			}

			if tt.err != nil {
				return
			}

			// If save is expected to succeed, then compare saved secret and delete after test
			res, err := client.Get(ctx, tt.secretName)
			require.NoError(t, err)
			if tt.update {
				require.Equal(t, res, updatedSecretValue)
			} else {
				require.Equal(t, res, secretValue)
			}

			err = client.Delete(ctx, tt.secretName)
			require.NoError(t, err)
		})
	}
}

func Test_Get(t *testing.T) {
	ctx := context.Background()

	secretValue, err := json.Marshal("test_secret_value")
	require.NoError(t, err)

	tests := []struct {
		testName   string
		secretName string
		save       bool
		err        error
	}{
		{"get-secret", secretName, true, nil},
		{"get-non-existent-secret", secretName, false, &secret.ErrNotFound{}},
		{"get-with-invalid-name", "", false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			client := Client{}
			if tt.save {
				err := client.Save(ctx, tt.secretName, secretValue)
				require.NoError(t, err)
			}

			res, err := client.Get(ctx, tt.secretName)
			require.Equal(t, err, tt.err)

			// If the get is successful then compare for values
			if tt.err == nil {
				require.Equal(t, res, secretValue)
			}

			// If secret is saved, cleanup secret at the end
			if tt.save {
				err = client.Delete(ctx, tt.secretName)
				require.NoError(t, err)
			}
		})
	}
}

func Test_Delete(t *testing.T) {
	ctx := context.Background()

	secretValue, err := json.Marshal("test_secret_value")
	require.NoError(t, err)

	tests := []struct {
		testName   string
		secretName string
		save       bool
		err        error
	}{
		{"delete-secret", secretName, true, nil},
		{"delete-non-existent-secret", secretName, false, &secret.ErrNotFound{}},
		{"delete-with-invalid-name", "", false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			client := Client{}
			if tt.save {
				err := client.Save(ctx, tt.secretName, secretValue)
				require.NoError(t, err)
			}

			err = client.Delete(ctx, tt.secretName)
			require.Equal(t, err, tt.err)
		})
	}
}
