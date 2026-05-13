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
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestWriteTerraformCLIConfig_ProviderInstallation(t *testing.T) {
	tests := []struct {
		name         string
		input        *datamodel.TerraformProviderInstallation
		wantPathSet  bool
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:        "nil input writes nothing",
			input:       nil,
			wantPathSet: false,
		},
		{
			name:        "empty installation block writes nothing",
			input:       &datamodel.TerraformProviderInstallation{},
			wantPathSet: false,
		},
		{
			name: "network mirror without URL is skipped",
			input: &datamodel.TerraformProviderInstallation{
				NetworkMirror: &datamodel.TerraformProviderMirror{
					Include: []string{"*"},
				},
			},
			wantPathSet: false,
		},
		{
			name: "network mirror with URL and patterns",
			input: &datamodel.TerraformProviderInstallation{
				NetworkMirror: &datamodel.TerraformProviderMirror{
					URL:     "https://mirror.corp.example.com/terraform/providers/",
					Include: []string{"*"},
					Exclude: []string{"hashicorp/azurerm"},
				},
			},
			wantPathSet: true,
			wantContains: []string{
				"provider_installation {",
				`network_mirror {`,
				`url = "https://mirror.corp.example.com/terraform/providers/"`,
				`include = ["*"]`,
				`exclude = ["hashicorp/azurerm"]`,
			},
			wantAbsent: []string{"direct {", "credentials "},
		},
		{
			name: "direct only with exclude",
			input: &datamodel.TerraformProviderInstallation{
				Direct: &datamodel.TerraformProviderDirect{
					Exclude: []string{"hashicorp/azurerm"},
				},
			},
			wantPathSet: true,
			wantContains: []string{
				`direct {`,
				`exclude = ["hashicorp/azurerm"]`,
			},
			wantAbsent: []string{"network_mirror {"},
		},
		{
			name: "both blocks rendered",
			input: &datamodel.TerraformProviderInstallation{
				NetworkMirror: &datamodel.TerraformProviderMirror{
					URL:     "https://mirror/",
					Include: []string{"*"},
				},
				Direct: &datamodel.TerraformProviderDirect{
					Exclude: []string{"hashicorp/azurerm", "hashicorp/aws"},
				},
			},
			wantPathSet: true,
			wantContains: []string{
				"network_mirror {",
				"direct {",
				`exclude = ["hashicorp/azurerm", "hashicorp/aws"]`,
			},
		},
		{
			name: "url with embedded quote is escaped",
			input: &datamodel.TerraformProviderInstallation{
				NetworkMirror: &datamodel.TerraformProviderMirror{
					URL: `https://example.com/"quoted"/`,
				},
			},
			wantPathSet: true,
			wantContains: []string{
				`url = "https://example.com/\"quoted\"/"`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := t.TempDir()
			path, err := writeTerraformCLIConfig(workingDir, tc.input, nil, nil)
			require.NoError(t, err)

			if !tc.wantPathSet {
				require.Empty(t, path)
				_, statErr := os.Stat(filepath.Join(workingDir, terraformCLIConfigFileName))
				require.True(t, os.IsNotExist(statErr), "expected no .terraformrc to be written")
				return
			}

			require.NotEmpty(t, path)
			require.Equal(t, filepath.Join(workingDir, terraformCLIConfigFileName), path)

			content, err := os.ReadFile(path)
			require.NoError(t, err)
			body := string(content)
			for _, want := range tc.wantContains {
				require.Contains(t, body, want)
			}
			for _, absent := range tc.wantAbsent {
				require.NotContains(t, body, absent)
			}

			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, terraformCLIConfigFileMode, info.Mode().Perm())
		})
	}
}

func TestWriteTerraformCLIConfig_BadDir(t *testing.T) {
	_, err := writeTerraformCLIConfig("/nonexistent/path/that/does/not/exist",
		&datamodel.TerraformProviderInstallation{
			NetworkMirror: &datamodel.TerraformProviderMirror{URL: "https://mirror/"},
		}, nil, nil)
	require.Error(t, err)
}

func TestWriteTerraformCLIConfig_Credentials(t *testing.T) {
	const (
		secretA = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/secretStores/tf-cloud"
		secretB = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/secretStores/corp-registry"
	)

	tests := []struct {
		name         string
		creds        map[string]datamodel.TerraformCredentialConfig
		secrets      map[string]recipes.SecretData
		wantErr      string
		wantContains []string
	}{
		{
			name: "single credentials block",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io": {Secret: secretA},
			},
			secrets: map[string]recipes.SecretData{
				secretA: {Type: "generic", Data: map[string]string{"token": "tfc-token-value"}},
			},
			wantContains: []string{
				`credentials "app.terraform.io" {`,
				`token = "tfc-token-value"`,
			},
		},
		{
			name: "two credentials blocks rendered in deterministic (sorted) order",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io":    {Secret: secretA},
				"reg.corp.example.io": {Secret: secretB},
			},
			secrets: map[string]recipes.SecretData{
				secretA: {Type: "generic", Data: map[string]string{"token": "tfc-token"}},
				secretB: {Type: "generic", Data: map[string]string{"token": "corp-token"}},
			},
			wantContains: []string{
				`credentials "app.terraform.io" {`,
				`credentials "reg.corp.example.io" {`,
			},
		},
		{
			name: "credential with empty secret reference fails",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io": {Secret: ""},
			},
			wantErr: "no secret reference",
		},
		{
			name: "credential with missing secret data fails",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io": {Secret: secretA},
			},
			secrets: map[string]recipes.SecretData{},
			wantErr: "no secret data was fetched",
		},
		{
			name: "credential with missing token key fails",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io": {Secret: secretA},
			},
			secrets: map[string]recipes.SecretData{
				secretA: {Type: "generic", Data: map[string]string{"pat": "wrong-key"}},
			},
			wantErr: `missing the "token" key`,
		},
		{
			name: "token with embedded quote is escaped",
			creds: map[string]datamodel.TerraformCredentialConfig{
				"app.terraform.io": {Secret: secretA},
			},
			secrets: map[string]recipes.SecretData{
				secretA: {Type: "generic", Data: map[string]string{"token": `tok"with"quotes`}},
			},
			wantContains: []string{`token = "tok\"with\"quotes"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := t.TempDir()
			path, err := writeTerraformCLIConfig(workingDir, nil, tc.creds, tc.secrets)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, path)

			body, err := os.ReadFile(path)
			require.NoError(t, err)
			for _, want := range tc.wantContains {
				require.Contains(t, string(body), want)
			}

			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, terraformCLIConfigFileMode, info.Mode().Perm())
		})
	}
}

func TestWriteTerraformCLIConfig_BothBlocks(t *testing.T) {
	const secretID = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/secretStores/tf-cloud"

	pi := &datamodel.TerraformProviderInstallation{
		NetworkMirror: &datamodel.TerraformProviderMirror{URL: "https://mirror/"},
	}
	creds := map[string]datamodel.TerraformCredentialConfig{
		"app.terraform.io": {Secret: secretID},
	}
	secrets := map[string]recipes.SecretData{
		secretID: {Type: "generic", Data: map[string]string{"token": "tok"}},
	}

	workingDir := t.TempDir()
	path, err := writeTerraformCLIConfig(workingDir, pi, creds, secrets)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(body)
	require.Contains(t, s, `provider_installation {`)
	require.Contains(t, s, `credentials "app.terraform.io" {`)
}
