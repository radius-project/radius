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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestAddSecretsToGitConfigIfApplicable(t *testing.T) {
	secretData := map[string]recipes.SecretData{
		"git-secret": {
			Data: map[string]string{
				"username": "test-user",
				"pat":      "ghp_token",
			},
		},
		"another-git-secret": {
			Data: map[string]string{
				"username": "another-user",
				"pat":      "another_token",
			},
		},
	}

	tests := []struct {
		desc             string
		config           recipes.Configuration
		secrets          map[string]recipes.SecretData
		expectedResponse []string
		shouldCreateGit  bool
		expectedErr      error
	}{
		{
			desc: "success with single host",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {Secret: "git-secret"},
								},
							},
						},
					},
				},
			},
			secrets:          secretData,
			expectedResponse: []string{"[url \"https://test-user:ghp_token@github.com\"]\n\tinsteadOf = https://github.com\n"},
			shouldCreateGit:  true,
			expectedErr:      nil,
		},
		{
			desc: "success with multiple hosts",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com":   {Secret: "git-secret"},
									"othertfs.com": {Secret: "another-git-secret"},
								},
							},
						},
					},
				},
			},
			secrets: secretData,
			expectedResponse: []string{
				"[url \"https://test-user:ghp_token@github.com\"]\n\tinsteadOf = https://github.com\n",
				"[url \"https://another-user:another_token@othertfs.com\"]\n\tinsteadOf = https://othertfs.com\n",
			},
			shouldCreateGit: true,
			expectedErr:     nil,
		},
		{
			desc: "no pat config",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: nil,
							},
						},
					},
				},
			},
			secrets:          secretData,
			expectedResponse: nil,
			shouldCreateGit:  false,
			expectedErr:      nil,
		},
		{
			desc: "secret not found",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {Secret: "non-existent-secret"},
								},
							},
						},
					},
				},
			},
			secrets:         secretData,
			shouldCreateGit: true,
			expectedErr:     fmt.Errorf("secrets not found for secret store ID %q", "non-existent-secret"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			err := addSecretsToGitConfigIfApplicable(context.Background(), tt.config, tt.secrets, tmpdir)
			if tt.expectedErr == nil {
				require.NoError(t, err)
				if tt.shouldCreateGit {
					fileContent, err := os.ReadFile(filepath.Join(tmpdir, ".git", "config"))
					require.NoError(t, err)
					for _, res := range tt.expectedResponse {
						require.Contains(t, string(fileContent), res)
					}
				} else {
					_, err := os.Stat(filepath.Join(tmpdir, ".git"))
					require.True(t, os.IsNotExist(err), ".git directory should not be created")
				}
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErr.Error())
			}
		})
	}
}

func TestSetGitConfigForDir(t *testing.T) {
	tests := []struct {
		desc             string
		workingDirectory string
		expectedResponse string
	}{
		{
			desc:             "success",
			workingDirectory: "test-working-dir",
			expectedResponse: "[includeIf \"gitdir:test-working-dir/\"]\n\tpath = test-working-dir/.git/config\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			config, err := withGlobalGitConfigFile(tmpdir, ``)
			require.NoError(t, err)
			defer config()
			err = setGitConfigForDir(tt.workingDirectory)
			require.NoError(t, err)
			fileContent, err := os.ReadFile(filepath.Join(tmpdir, ".gitconfig"))
			require.NoError(t, err)
			require.Contains(t, string(fileContent), tt.expectedResponse)

		})
	}
}

func TestUnsetGitConfigForDir(t *testing.T) {
	tests := []struct {
		desc             string
		workingDirectory string
		templatePath     string
		fileContent      string
	}{
		{
			desc:             "success",
			workingDirectory: "test-working-dir",
			templatePath:     "git::https://github.com/project/module",
			fileContent: `
			[includeIf "gitdir:test-working-dir/"]
        path = test-working-dir/.git/config
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			config, err := withGlobalGitConfigFile(tmpdir, tt.fileContent)
			require.NoError(t, err)
			defer config()
			err = unsetGitConfigForDir(tt.workingDirectory)
			require.NoError(t, err)
			contents, err := os.ReadFile(filepath.Join(tmpdir, ".gitconfig"))
			require.NoError(t, err)
			require.NotContains(t, string(contents), tt.fileContent)
		})
	}
}

func getSecretList() map[string]string {
	secrets := map[string]string{
		"username": "test-user",
		"pat":      "ghp_token",
	}

	return secrets
}

func TestConfigureSSHAuth(t *testing.T) {
	tests := []struct {
		name                  string
		privateKey            string
		strictHostKeyChecking string
		expectError           bool
		expectSSHCommand      string
	}{
		{
			name:                  "SSH auth with strict host key checking enabled",
			privateKey:            "-----BEGIN OPENSSH PRIVATE KEY-----\ntest-key-content\n-----END OPENSSH PRIVATE KEY-----",
			strictHostKeyChecking: "true",
			expectError:           false,
			expectSSHCommand:      "StrictHostKeyChecking=yes",
		},
		{
			name:                  "SSH auth with strict host key checking disabled",
			privateKey:            "-----BEGIN OPENSSH PRIVATE KEY-----\ntest-key-content\n-----END OPENSSH PRIVATE KEY-----",
			strictHostKeyChecking: "false",
			expectError:           false,
			expectSSHCommand:      "StrictHostKeyChecking=no",
		},
		{
			name:                  "SSH auth with default strict host key checking",
			privateKey:            "-----BEGIN OPENSSH PRIVATE KEY-----\ntest-key-content\n-----END OPENSSH PRIVATE KEY-----",
			strictHostKeyChecking: "",
			expectError:           false,
			expectSSHCommand:      "StrictHostKeyChecking=yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir := t.TempDir()

			secrets := map[string]string{
				"privateKey": tt.privateKey,
			}
			if tt.strictHostKeyChecking != "" {
				secrets["strictHostKeyChecking"] = tt.strictHostKeyChecking
			}

			// Initialize git repository first
			err := os.MkdirAll(filepath.Join(tmpdir, ".git"), 0755)
			require.NoError(t, err)

			// Create an empty git config file
			err = os.WriteFile(filepath.Join(tmpdir, ".git", "config"), []byte(""), 0644)
			require.NoError(t, err)
			err = configureSSHAuth(tmpdir, tt.privateKey, secrets)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify SSH key file was created
				sshKeyPath := filepath.Join(tmpdir, ".ssh", "id_rsa")
				require.FileExists(t, sshKeyPath)

				// Verify key content
				keyContent, err := os.ReadFile(sshKeyPath)
				require.NoError(t, err)
				require.Equal(t, tt.privateKey, string(keyContent))

				// Verify file permissions
				info, err := os.Stat(sshKeyPath)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), info.Mode())

				// Verify git config contains SSH command
				gitConfigPath := filepath.Join(tmpdir, ".git", "config")
				gitConfigContent, err := os.ReadFile(gitConfigPath)
				require.NoError(t, err)
				require.Contains(t, string(gitConfigContent), tt.expectSSHCommand)
			}
		})
	}
}

func Test_GetGitURL(t *testing.T) {
	tests := []struct {
		desc         string
		templatePath string
		expectedURL  string
		expectedErr  bool
	}{
		{
			desc:         "success",
			templatePath: "git::dev.azure.com/project/module",
			expectedURL:  "https://dev.azure.com/project/module",
			expectedErr:  false,
		},
		{
			desc:         "invalid url",
			templatePath: "git::https://dev.az  ure.com/project/module",
			expectedErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			url, err := GetGitURL(tt.templatePath)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, tt.expectedURL, url.String())
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestUnsetGitConfigForDirIfApplicable(t *testing.T) {
	tests := []struct {
		desc     string
		config   recipes.Configuration
		hasError bool
	}{
		{
			desc: "success with PAT config",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {Secret: "git-secret"},
								},
							},
						},
					},
				},
			},
			hasError: false,
		},
		{
			desc: "no PAT config - should not error",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: nil,
							},
						},
					},
				},
			},
			hasError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tmpDir := t.TempDir()
			ctx := context.Background()

			err := unsetGitConfigForDirIfApplicable(ctx, test.config, tmpDir)
			if test.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// withGlobalGitConfigFile creates a temporary git config file and sets it as the global config
func withGlobalGitConfigFile(tmpdir, content string) (func(), error) {
	gitConfigPath := filepath.Join(tmpdir, ".gitconfig")

	// Write the content to the temporary git config file
	if err := os.WriteFile(gitConfigPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write git config file: %w", err)
	}

	// Set the GIT_CONFIG_GLOBAL environment variable to point to our temporary file
	originalConfig := os.Getenv("GIT_CONFIG_GLOBAL")
	os.Setenv("GIT_CONFIG_GLOBAL", gitConfigPath)

	// Return cleanup function
	cleanup := func() {
		if originalConfig != "" {
			os.Setenv("GIT_CONFIG_GLOBAL", originalConfig)
		} else {
			os.Unsetenv("GIT_CONFIG_GLOBAL")
		}
	}

	return cleanup, nil
}
