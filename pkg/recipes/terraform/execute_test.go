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
	reflect "reflect"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	dm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	terraformGetTestHelper = "RADIUS_TERRAFORM_GET_TEST_HELPER"
	terraformCommandLog    = "RADIUS_TERRAFORM_COMMAND_LOG"
)

func TestMain(m *testing.M) {
	if os.Getenv(terraformGetTestHelper) == "1" {
		if commandLog := os.Getenv(terraformCommandLog); commandLog != "" && len(os.Args) > 1 {
			file, err := os.OpenFile(commandLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				os.Exit(1)
			}
			if _, err := file.WriteString(os.Args[1] + "\n"); err != nil {
				_ = file.Close()
				os.Exit(1)
			}
			if err := file.Close(); err != nil {
				os.Exit(1)
			}
		}

		moduleDir := filepath.Join(".terraform", "modules", "test-recipe")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			os.Exit(1)
		}
		if err := os.WriteFile(
			filepath.Join(moduleDir, "outputs.tf"),
			[]byte(`output "endpoint" { value = "declared" }`),
			0644); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestDeleteSkipsOutputMappingValidation(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalDir)
	t.Setenv(terraformGetTestHelper, "1")
	commandLog := filepath.Join(t.TempDir(), "terraform-commands.log")
	t.Setenv(terraformCommandLog, commandLog)

	require.NoError(t, os.Symlink(os.Args[0], filepath.Join(globalDir, "terraform")))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, ".terraform-ready"), nil, 0644))

	globalTerraformMutex.Lock()
	previousGlobalTerraformReady := globalTerraformReady
	globalTerraformReady = true
	globalTerraformMutex.Unlock()
	t.Cleanup(func() {
		globalTerraformMutex.Lock()
		globalTerraformReady = previousGlobalTerraformReady
		globalTerraformMutex.Unlock()
	})

	resourceRecipe := &recipes.ResourceMetadata{
		Name:          "widget",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/widgets/widget",
		EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/test-env",
	}
	kubernetesClient := fake.NewSimpleClientset()
	kubernetesClient.Fake.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		getAction := action.(k8stesting.GetAction)
		return true, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      getAction.GetName(),
				Namespace: backends.RadiusNamespace,
			},
		}, nil
	})
	kubernetesClient.Fake.PrependReactor("delete", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	kubernetesClients := kubernetesclientprovider.KubernetesClientProvider{}
	kubernetesClients.SetClientGoClient(kubernetesClient)

	e := executor{kubernetesClients: kubernetesClients}
	err := e.Delete(t.Context(), Options{
		RootDir:   t.TempDir(),
		EnvConfig: &recipes.Configuration{},
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
			ResourceType: "Test.Resources/widgets",
			Outputs:      map[string]string{"host": "missing"},
		},
		ResourceRecipe: resourceRecipe,
	})
	require.NoError(t, err)

	commands, err := os.ReadFile(commandLog)
	require.NoError(t, err)
	require.Contains(t, string(commands), "destroy\n")
}

func TestGenerateConfig(t *testing.T) {
	configTests := []struct {
		name       string
		workingDir string
		opts       Options
		err        string
	}{
		{
			name: "empty recipe name error",
			opts: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					TemplatePath: "test/module/source",
				},
				ResourceRecipe: &recipes.ResourceMetadata{
					ConnectedResourcesProperties: map[string]recipes.ConnectedResource{
						"conn1": {
							ID:   "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Datastores/redis/redis",
							Name: "redis",
							Type: "Applications.Datastores/redis",
							Properties: map[string]any{
								"dbName": "db",
							},
						},
					},
				},
			},
			err: ErrRecipeNameEmpty.Error(),
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			if tc.workingDir == "" {
				tc.workingDir = t.TempDir()
			}
			tf, err := tfexec.NewTerraform(tc.workingDir, filepath.Join(tc.workingDir, "terraform"))
			require.NoError(t, err)

			e := executor{}
			_, err = e.generateConfig(ctx, tf, tc.opts, requireValidOutputMappings)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestGenerateConfigRejectsInvalidOutputMappingBeforeProviderSetup(t *testing.T) {
	workingDir := t.TempDir()
	tf, err := tfexec.NewTerraform(workingDir, os.Args[0])
	require.NoError(t, err)
	require.NoError(t, tf.SetEnv(map[string]string{terraformGetTestHelper: "1"}))

	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
			ResourceType: "Test.Resources/widgets",
			Outputs:      map[string]string{"host": "missing"},
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	e := executor{}
	_, err = e.generateConfig(t.Context(), tf, options, requireValidOutputMappings)
	require.EqualError(t, err, `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["host"] -> "missing"; available module outputs: "endpoint"`)
}

func TestValidateOutputMappings(t *testing.T) {
	definition := &recipes.EnvironmentDefinition{
		Name:          "service-bus",
		ResourceType:  "Applications.Messaging/rabbitMQQueues",
		Outputs:       map[string]string{"host": "endpoint"},
		SecretOutputs: map[string]string{"connectionString": "primaryConnectionString"},
	}
	module := &moduleInspectResult{
		OutputSensitivity: map[string]bool{
			"endpoint": false,
		},
	}

	err := validateOutputMappings(definition, module, requireValidOutputMappings)
	require.EqualError(t, err, `recipe "service-bus" for resource type "Applications.Messaging/rabbitMQQueues": invalid outputs mapping: no declared module output matches secrets["connectionString"] -> "primaryConnectionString"; available module outputs: "endpoint"`)

	err = validateOutputMappings(definition, module, skipOutputMappingValidation)
	require.NoError(t, err)
}

func TestUsesOutputMappings(t *testing.T) {
	definition := &recipes.EnvironmentDefinition{
		Outputs: map[string]string{"host": "endpoint"},
	}

	tests := []struct {
		name           string
		hasResult      bool
		validationMode outputMappingValidationMode
		expected       bool
	}{
		{
			name:           "deploy uses mappings instead of result",
			hasResult:      true,
			validationMode: requireValidOutputMappings,
			expected:       true,
		},
		{
			name:           "delete preserves result behavior",
			hasResult:      true,
			validationMode: skipOutputMappingValidation,
			expected:       false,
		},
		{
			name:           "direct module deletion keeps mapped outputs",
			validationMode: skipOutputMappingValidation,
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := &moduleInspectResult{ResultOutputExists: tt.hasResult}
			require.Equal(t, tt.expected, usesOutputMappings(definition, module, tt.validationMode))
		})
	}
}

func Test_GetTerraformConfig(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	expectedConfig := config.TerraformConfig{
		Module: map[string]config.TFModuleConfig{
			"test-recipe": {"source": "test/module/source"}},
	}
	tfConfig, err := getTerraformConfig(t.Context(), testDir, options)
	require.NoError(t, err)
	require.Equal(t, &expectedConfig, tfConfig)
}

func Test_GetTerraformConfig_EmptyRecipeName(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	_, err := getTerraformConfig(t.Context(), testDir, options)
	require.Error(t, err)
	require.Equal(t, err, ErrRecipeNameEmpty)
}

func Test_GetTerraformConfig_InvalidDirectory(t *testing.T) {
	workingDir := "invalid-directory"
	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	_, err := getTerraformConfig(t.Context(), workingDir, options)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file: open invalid-directory/main.tf.json: no such file or directory")
}

func TestSetEnvironmentVariables(t *testing.T) {
	testCase := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name: "set environment variables",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: map[string]string{
								"TEST_ENV_VAR1": "value1",
								"TEST_ENV_VAR2": "value2",
							},
						},
					},
				},
			},
		},
		{
			name: "set environment variables with secrets",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: map[string]string{
								"TEST_ENV_VAR1": "value1",
								"TEST_ENV_VAR2": "value2",
							},
						},
						EnvSecrets: map[string]dm.SecretReference{
							"TEST_ENV_VAR3": {
								Source: "secretstoreid1",
								Key:    "secretkey1",
							},
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secretstoreid1": {
						Type: "generic",
						Data: map[string]string{"secretkey1": "secretvalue1"},
					},
				},
			},
		},
		{
			name: "missing secret keys",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: map[string]string{
								"TEST_ENV_VAR1": "value1",
								"TEST_ENV_VAR2": "value2",
							},
						},
						EnvSecrets: map[string]dm.SecretReference{
							"TEST_ENV_VAR3": {
								Source: "secretstoreid1",
								Key:    "secretkey1",
							},
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secretstoreid2": {
						Type: "generic",
						Data: map[string]string{"secretkey2": "secretvalue2"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing secret data",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						EnvSecrets: map[string]dm.SecretReference{
							"TEST_ENV_VAR3": {
								Source: "secretstoreid1",
								Key:    "secretkey1",
							},
						},
					},
				},
				Secrets: map[string]recipes.SecretData{
					"secretstoreid2": {
						Type: "generic",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "AdditionalProperties set to nil",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: nil,
						},
					},
				},
			},
		},
		{
			name: "no environment variables",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{},
				},
			},
		},
		{
			name: "provider_installation writes .terraformrc and sets TF_CLI_CONFIG_FILE",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Terraform: dm.TerraformConfigProperties{
							ProviderInstallation: &dm.TerraformProviderInstallation{
								NetworkMirror: &dm.TerraformProviderMirror{
									URL:     "https://mirror.example.com/",
									Include: []string{"*"},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := t.TempDir()

			tf, err := tfexec.NewTerraform(workingDir, filepath.Join(workingDir, "terraform"))
			require.NoError(t, err)

			e := executor{}
			err = e.setEnvironmentVariables(tf, tc.opts)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// When provider_installation is set, the helper should have written .terraformrc
			// into the working directory.
			if tc.opts.EnvConfig != nil && tc.opts.EnvConfig.RecipeConfig.Terraform.ProviderInstallation != nil {
				_, statErr := os.Stat(filepath.Join(workingDir, terraformCLIConfigFileName))
				require.NoError(t, statErr, "expected .terraformrc to be written to working dir")
			}
		})
	}
}

func TestApplyTerraformCLIConfig(t *testing.T) {
	tests := []struct {
		name      string
		opts      Options
		wantFile  bool
		wantError bool
	}{
		{
			name:     "nil EnvConfig is a no-op",
			opts:     Options{},
			wantFile: false,
		},
		{
			name: "nil ProviderInstallation is a no-op",
			opts: Options{
				EnvConfig: &recipes.Configuration{},
			},
			wantFile: false,
		},
		{
			name: "writes .terraformrc when ProviderInstallation is set",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Terraform: dm.TerraformConfigProperties{
							ProviderInstallation: &dm.TerraformProviderInstallation{
								NetworkMirror: &dm.TerraformProviderMirror{
									URL: "https://mirror.example.com/",
								},
							},
						},
					},
				},
			},
			wantFile: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := t.TempDir()
			tf, err := tfexec.NewTerraform(workingDir, filepath.Join(workingDir, "terraform"))
			require.NoError(t, err)

			e := executor{}
			err = e.applyTerraformCLIConfig(tf, tc.opts)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			path := filepath.Join(workingDir, terraformCLIConfigFileName)
			_, statErr := os.Stat(path)
			if tc.wantFile {
				require.NoError(t, statErr)
			} else {
				require.True(t, os.IsNotExist(statErr))
			}
		})
	}
}

func TestSplitEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envVars []string
		want    map[string]string
	}{
		{
			name:    "nil input",
			envVars: nil,
			want:    map[string]string{},
		},
		{
			name:    "empty input",
			envVars: []string{},
			want:    map[string]string{},
		},
		{
			name:    "single variable",
			envVars: []string{"VAR1=value1"},
			want:    map[string]string{"VAR1": "value1"},
		},
		{
			name:    "multiple variables",
			envVars: []string{"VAR1=value1", "VAR2=value2"},
			want:    map[string]string{"VAR1": "value1", "VAR2": "value2"},
		},
		{
			name:    "variable with no value",
			envVars: []string{"VAR1="},
			want:    map[string]string{"VAR1": ""},
		},
		{
			name:    "variable with equals sign in value",
			envVars: []string{"VAR1=value1=value2"},
			want:    map[string]string{"VAR1": "value1=value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitEnvVar(tt.envVars); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitEnvVar() = %v, want %v", got, tt.want)
			}
		})
	}
}
