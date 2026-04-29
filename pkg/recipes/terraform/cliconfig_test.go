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
	"github.com/stretchr/testify/require"
)

func TestWriteTerraformCLIConfig(t *testing.T) {
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
			wantAbsent: []string{"direct {"},
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
			path, err := writeTerraformCLIConfig(workingDir, tc.input)
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
		})
	require.Error(t, err)
}
