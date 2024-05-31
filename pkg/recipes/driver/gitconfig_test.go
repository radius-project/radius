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
			expectedResponse: "[url \"https://test-user:ghp_token@github.com\"]\n\tinsteadOf = https://github.com\n",
			expectedErr:      nil,
		},
		{
			desc:         "invalid git url",
			templatePath: "git::https://gith  ub.com/project/module",
			expectedErr:  errors.New("failed to parse git url"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			_, recipeMetadata, _ := buildTestInputs()
			if tt.desc == "invalid resource id" {
				recipeMetadata.EnvironmentID = "//planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env"
			}
			err := addSecretsToGitConfig(tmpdir, getSecretList(), tt.templatePath)
			if tt.expectedErr == nil {
				require.NoError(t, err)
				fileContent, err := os.ReadFile(tmpdir + "/.git/config")
				require.NoError(t, err)
				require.Contains(t, string(fileContent), tt.expectedResponse)
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
			err = unsetGitConfigForDir(tt.workingDirectory, getSecretList(), tt.templatePath)
			require.NoError(t, err)
			contents, err := os.ReadFile(filepath.Join(tmpdir, ".gitconfig"))
			require.NoError(t, err)
			require.NotContains(t, string(contents), tt.fileContent)
		})
	}
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
