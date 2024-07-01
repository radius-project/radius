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

package terraform

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestNewTerraform_Success(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")
	expectedWorkingDir := filepath.Join(testDir, executionSubDir)

	tf, err := NewTerraform(testcontext.New(t), testDir, execPath)
	require.NoError(t, err)
	require.Equal(t, expectedWorkingDir, tf.WorkingDir())
}

func TestNewTerraform_InvalidDir(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	// Create a read-only directory within the temporary directory.
	readOnlyDir := filepath.Join(testDir, "read-only-dir")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)

	execPath := filepath.Join(testDir, "terraform")

	// Call NewTerraform with read only root directory.
	_, err = NewTerraform(testcontext.New(t), readOnlyDir, execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create working directory for terraform execution")
}

func TestNewTerraform_EmptyExecPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	// Call NewTerraform with an empty exec path.
	_, err := NewTerraform(testcontext.New(t), testDir, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize Terraform: no suitable terraform binary could be found")
}

func TestCreateWorkingDir_Created(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	expectedWorkingDir := filepath.Join(testDir, executionSubDir)
	workingDir, err := createWorkingDir(testcontext.New(t), testDir)
	require.NoError(t, err)
	require.Equal(t, expectedWorkingDir, workingDir)

	// Assert that the working directory was created.
	_, err = os.Stat(workingDir)
	require.NoError(t, err)
}

func TestCreateWorkingDir_Error(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	// Create a read-only directory within the temporary directory.
	readOnlyDir := filepath.Join(testDir, "read-only-dir")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)

	// Call createWorkingDir with the read-only directory.
	_, err = createWorkingDir(testcontext.New(t), readOnlyDir)

	// Assert that createWorkingDir returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create working directory")
}

// TestGetProviderEnvSecretIDs tests the GetProviderEnvSecretIDs function which is a wrapper around the
// extractProviderSecretIDs and extractEnvSecretIDs functions.
func TestGetProviderEnvSecretIDs(t *testing.T) {
	tests := []struct {
		name      string
		envConfig recipes.Configuration
		want      map[string][]string
	}{
		{
			name: "both env and provider secrets populated",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {
								{
									Secrets: map[string]datamodel.SecretReference{
										"aws_secret1": {Source: "my-app-secret-source-id", Key: "secret-key1"},
									},
								},
							},
						},
					},
					EnvSecrets: map[string]datamodel.SecretReference{
						"env_secret1": {Source: "my-env-secret-source-id", Key: "secret-key2"},
					},
				},
			},
			want: map[string][]string{
				"my-app-secret-source-id": {"secret-key1"},
				"my-env-secret-source-id": {"secret-key2"},
			},
		},
		{
			name: "provider secret populated",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {
								{
									Secrets: map[string]datamodel.SecretReference{
										"aws_secret1": {Source: "my-app-secret-source-id", Key: "secret-key1"},
									},
								},
							},
						},
					},
				},
			},
			want: map[string][]string{
				"my-app-secret-source-id": {"secret-key1"},
			},
		},
		{
			name: "env secret populated",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					EnvSecrets: map[string]datamodel.SecretReference{
						"env_secret1": {Source: "my-env-secret-source-id", Key: "secret-key-env"},
					},
				},
			},
			want: map[string][]string{
				"my-env-secret-source-id": {"secret-key-env"},
			},
		},
		{
			name: "secrets are declared nil",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {
								{
									Secrets: nil,
								},
							},
						},
					},
					EnvSecrets: nil,
				},
			},
			want: map[string][]string{},
		},
		{
			name: "secrets are nil",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {},
						},
					},
				},
			},
			want: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerSecretIDs := GetProviderEnvSecretIDs(tt.envConfig)
			require.Equal(t, tt.want, providerSecretIDs)
		})
	}
}

func TestAddSecretKeys(t *testing.T) {
	tests := []struct {
		name           string
		secrets        map[string][]string
		secretStoreID  string
		key            string
		expectedResult map[string][]string
	}{
		{
			name:           "Add to empty map",
			secrets:        make(map[string][]string),
			secretStoreID:  "store1",
			key:            "key1",
			expectedResult: map[string][]string{"store1": {"key1"}},
		},
		{
			name:           "Add new key to existing store",
			secrets:        map[string][]string{"store1": {"key1"}},
			secretStoreID:  "store1",
			key:            "key2",
			expectedResult: map[string][]string{"store1": {"key1", "key2"}},
		},
		{
			name:           "Add key to new store",
			secrets:        map[string][]string{"store1": {"key1"}},
			secretStoreID:  "store2",
			key:            "key1",
			expectedResult: map[string][]string{"store1": {"key1"}, "store2": {"key1"}},
		},
		{
			name:           "Ignore empty secretStoreID",
			secrets:        map[string][]string{"store1": {"key1"}},
			secretStoreID:  "",
			key:            "key1",
			expectedResult: map[string][]string{"store1": {"key1"}},
		},
		{
			name:           "Ignore empty key",
			secrets:        map[string][]string{"store1": {"key1"}},
			secretStoreID:  "store1",
			key:            "",
			expectedResult: map[string][]string{"store1": {"key1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu := &sync.Mutex{}
			addSecretKeys(tt.secrets, tt.secretStoreID, tt.key, mu)
			require.Equal(t, tt.expectedResult, tt.secrets)
		})
	}
}
