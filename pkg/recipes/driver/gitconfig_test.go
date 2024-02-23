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

package driver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestAddConfig(t *testing.T) {
	tests := []struct {
		desc             string
		templatePath     string
		expectedResponse string
		expectedErr      error
	}{
		{
			desc:             "success",
			templatePath:     "git::https://github.com/project/module",
			expectedResponse: "[url \"https://test-user:ghp_token@github.com\"]\n\tinsteadOf = https://env1-app1-test-redis-recipe-github.com\n",
			expectedErr:      nil,
		},
		{
			desc:         "invalid git url",
			templatePath: "git::https://gith  ub.com/project/module",
			expectedErr:  errors.New("failed to parse git url"),
		},
		{
			desc:         "invalid resource id",
			templatePath: "git::https://github.com/project/module",
			expectedErr:  errors.New(" is not a valid resource id"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			config, err := withGlobalGitConfigFile(tmpdir, ``)
			require.NoError(t, err)
			defer config()
			_, recipeMetadata, _ := buildTestInputs()
			if tt.desc == "invalid resource id" {
				recipeMetadata.EnvironmentID = "//planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env"
			}
			err = addSecretsToGitConfig(getSecretList(), &recipeMetadata, tt.templatePath)
			if tt.expectedErr == nil {
				require.NoError(t, err)
				fileContent, err := os.ReadFile(filepath.Join(tmpdir, ".gitconfig"))
				require.NoError(t, err)
				require.Contains(t, string(fileContent), tt.expectedResponse)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErr.Error())
			}
		})
	}

}
func TestUnsetConfig(t *testing.T) {
	tests := []struct {
		desc             string
		templatePath     string
		fileContent      string
		expectedResponse string
		expectedErr      error
	}{
		{
			desc:         "success",
			templatePath: "git::https://github.com/project/module",
			fileContent: `
						[url "https://test-user:ghp_token@github.com"]
							insteadOf = https://env1-app1-test-redis-recipe-github.com
						`,
			expectedErr: nil,
		},
		{
			desc:         "invalid url",
			templatePath: "git::https://git hub.com/project/module",
			fileContent: `
						[url "https://test-user:ghp_token@github.com"]
							insteadOf = https://env1-app1-test-redis-recipe-github.com
						`,
			expectedErr: errors.New("failed to parse git url"),
		},
		{
			desc:         "empty config file",
			templatePath: "git::https://github.com/project/module",
			fileContent:  "",
			expectedErr:  errors.New("failed to unset git config"),
		},
	}
	for _, tt := range tests {
		tmpdir := t.TempDir()
		config, err := withGlobalGitConfigFile(tmpdir, tt.fileContent)
		require.NoError(t, err)
		defer config()
		err = unsetSecretsFromGitConfig(getSecretList(), tt.templatePath)
		if tt.expectedErr == nil {
			require.NoError(t, err)
			contents, err := os.ReadFile(filepath.Join(tmpdir, ".gitconfig"))
			require.NoError(t, err)
			require.NotContains(t, string(contents), tt.fileContent)
		} else {
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedErr.Error())
		}
	}
}

func withGlobalGitConfigFile(tmpdir string, content string) (func(), error) {

	tmpGitConfigFile := filepath.Join(tmpdir, ".gitconfig")

	err := os.WriteFile(
		tmpGitConfigFile,
		[]byte(content),
		0777,
	)

	if err != nil {
		return func() {}, err
	}
	prevGitConfigEnv := os.Getenv("HOME")
	os.Setenv("HOME", tmpdir)

	return func() {
		os.Setenv("HOME", prevGitConfigEnv)
	}, nil
}

func getSecretList() v20231001preview.SecretStoresClientListSecretsResponse {
	secrets := v20231001preview.SecretStoresClientListSecretsResponse{
		SecretStoreListSecretsResult: v20231001preview.SecretStoreListSecretsResult{
			Data: map[string]*v20231001preview.SecretValueProperties{
				"username": {
					Value: to.Ptr("test-user"),
				},
				"pat": {
					Value: to.Ptr("ghp_token"),
				},
			},
		},
	}
	return secrets
}
