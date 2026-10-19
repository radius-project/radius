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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	reflect "reflect"
	goruntime "runtime"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	dm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/retry"
	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	terraformGetTestHelper = "RADIUS_TERRAFORM_GET_TEST_HELPER"
	terraformCommandLog    = "RADIUS_TERRAFORM_COMMAND_LOG"
)

func TestMain(m *testing.M) {
	exitCode := 0
	if os.Getenv(terraformGetTestHelper) == "1" {
		if err := runTerraformTestHelper(); err != nil {
			fmt.Fprintf(os.Stderr, "terraform test helper failed: %v\n", err)
			exitCode = 1
		}
	} else {
		exitCode = m.Run()
	}

	os.Exit(exitCode) //nolint:forbidigo // This is OK inside the TestMain function.
}

func runTerraformTestHelper() error {
	if os.Getenv(terraformBackendSnapshots) != "" {
		if err := recordBackendTestExecution(); err != nil {
			return err
		}
	}
	if commandLog := os.Getenv(terraformCommandLog); commandLog != "" && len(os.Args) > 1 {
		file, err := os.OpenFile(commandLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening command log: %w", err)
		}
		if _, err := file.WriteString(os.Args[1] + "\n"); err != nil {
			_ = file.Close()
			return fmt.Errorf("writing command log: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("closing command log: %w", err)
		}
	}

	moduleDir := filepath.Join(".terraform", "modules", "test-recipe")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("creating test module directory: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(moduleDir, "outputs.tf"),
		[]byte(`output "endpoint" { value = "declared" }`),
		0644); err != nil {
		return fmt.Errorf("writing test module outputs: %w", err)
	}
	if os.Getenv("RADIUS_TERRAFORM_CONTEXT_VARIABLE") == "1" {
		if err := os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(`variable "context" { type = any }`), 0600); err != nil {
			return fmt.Errorf("writing test module context variable: %w", err)
		}
	}
	if provider := os.Getenv("RADIUS_TERRAFORM_REQUIRED_PROVIDER"); provider != "" {
		content := fmt.Sprintf("terraform {\n required_providers {\n %s = {\n source = \"hashicorp/%s\"\n", provider, provider)
		if alias := os.Getenv("RADIUS_TERRAFORM_PROVIDER_ALIAS"); alias != "" {
			content += fmt.Sprintf(" configuration_aliases = [%s.%s]\n", provider, alias)
		}
		content += " }\n }\n}\n"
		if err := os.WriteFile(filepath.Join(moduleDir, "providers.tf"), []byte(content), 0600); err != nil {
			return err
		}
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Fprint(os.Stdout, `{"terraform_version":"1.15.8","platform":"test","provider_selections":{}}`)
		case "show":
			fmt.Fprint(os.Stdout, `{"format_version":"1.0","terraform_version":"1.15.8","values":{"outputs":{"endpoint":{"value":"declared","type":"string","sensitive":false}}}}`)
		case "output":
			output := os.Getenv("RADIUS_TERRAFORM_TEST_OUTPUT")
			if output == "" {
				output = "{}"
			}
			fmt.Fprint(os.Stdout, output)
		}
	}
	return nil
}

func TestRetryTerraformInitRetriesTransientProviderDiscoveryFailure(t *testing.T) {
	retryer := retry.NewRetryer(&retry.RetryConfig{
		BackoffStrategy: goretry.WithMaxRetries(2, goretry.NewConstant(time.Nanosecond)),
	})

	attempts := 0
	err := retryTerraformInit(t.Context(), retryer, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("failed to\nrequest discovery document: Client.Timeout\nexceeded while awaiting headers")
		}

		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestRetryTerraformInitDoesNotRetryPermanentFailure(t *testing.T) {
	retryer := retry.NewRetryer(&retry.RetryConfig{
		BackoffStrategy: goretry.WithMaxRetries(2, goretry.NewConstant(time.Nanosecond)),
	})

	attempts := 0
	expectedErr := fmt.Errorf("failed to query available provider packages: provider does not exist")
	err := retryTerraformInit(t.Context(), retryer, func(context.Context) error {
		attempts++
		return expectedErr
	})

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, attempts)
}

func TestRetryTerraformInitDoesNotRetryCancellation(t *testing.T) {
	retryer := retry.NewRetryer(&retry.RetryConfig{
		BackoffStrategy: goretry.WithMaxRetries(2, goretry.NewConstant(time.Nanosecond)),
	})

	attempts := 0
	expectedErr := fmt.Errorf("failed to request discovery document: %w", context.Canceled)
	err := retryTerraformInit(t.Context(), retryer, func(context.Context) error {
		attempts++
		return expectedErr
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestIsTransientTerraformInitError(t *testing.T) {
	tests := []struct {
		name      string
		signature string
	}{
		{name: "client timeout", signature: "Client.Timeout exceeded"},
		{name: "context deadline", signature: "context deadline exceeded"},
		{name: "connection refused", signature: "connection refused"},
		{name: "connection reset", signature: "connection reset"},
		{name: "DNS failure", signature: "no such host"},
		{name: "temporary DNS failure", signature: "temporary failure in name resolution"},
		{name: "TLS timeout", signature: "TLS handshake timeout"},
		{name: "truncated response", signature: "unexpected EOF"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New("failed to request discovery document: " + tc.signature)
			require.True(t, isTransientTerraformInitError(err))
		})
	}
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
			Name:      getAction.GetName(),
			Namespace: backends.RadiusNamespace,
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

func TestInitAndDestroyRestoresRecipeInputs(t *testing.T) {
	inputs := map[string]any{
		"context": map[string]any{
			"resource": map[string]any{
				"properties": map[string]any{
					"data": map[string]any{
						"password": map[string]any{"value": "previous-secret"},
					},
				},
			},
		},
	}
	saved := &config.TerraformConfig{Module: map[string]config.TFModuleConfig{"recipe": config.TFModuleConfig(inputs)}}
	require.NoError(t, saved.AddRecipeInputs("recipe"))
	encoded := saved.Output[config.RecipeInputsOutputName].(map[string]any)["value"]

	savedOutputs := map[string]any{
		config.RecipeInputsOutputName: map[string]any{"value": encoded, "type": "string", "sensitive": true},
	}

	for _, tt := range []struct {
		name        string
		outputs     map[string]any
		failCommand string
		wantError   string
		restored    bool
		noDestroy   bool
		wantRefresh bool
	}{
		{name: "saved inputs", outputs: savedOutputs, restored: true},
		{name: "legacy state", outputs: map[string]any{}},
		{name: "invalid output type", outputs: map[string]any{
			config.RecipeInputsOutputName: map[string]any{"value": 42, "type": "number"},
		}, wantError: "expected a string", noDestroy: true},
		{name: "invalid saved inputs", outputs: map[string]any{
			config.RecipeInputsOutputName: map[string]any{"value": base64.StdEncoding.EncodeToString([]byte("secret")), "type": "string"},
		}, wantError: "expected a JSON object", noDestroy: true},
		{name: "output failure", outputs: map[string]any{}, failCommand: "output", wantError: "reading terraform recipe inputs for deletion", noDestroy: true},
		{name: "destroy failure preserves saved inputs", outputs: savedOutputs, failCommand: "destroy", wantError: "terraform destroy failure", restored: true, wantRefresh: true},
		{name: "destroy failure with legacy state", outputs: map[string]any{}, failCommand: "destroy", wantError: "terraform destroy failure"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := t.TempDir()
			cfg := &config.TerraformConfig{Module: map[string]config.TFModuleConfig{
				"recipe": {
					"source": "test/module",
					"context": map[string]any{
						"resource": map[string]any{
							"properties": map[string]any{
								"data": map[string]any{"password": map[string]any{"value": nil}},
							},
						},
					},
				},
			}}
			require.NoError(t, cfg.AddRecipeInputs("recipe"))
			require.NoError(t, cfg.Save(t.Context(), workingDir))
			snapshotsPath := filepath.Join(t.TempDir(), "executions.jsonl")
			outputs, err := json.Marshal(tt.outputs)
			require.NoError(t, err)
			tf, err := tfexec.NewTerraform(workingDir, os.Args[0])
			require.NoError(t, err)
			env := map[string]string{
				terraformGetTestHelper:         "1",
				terraformBackendSnapshots:      snapshotsPath,
				"RADIUS_TERRAFORM_TEST_OUTPUT": string(outputs),
			}
			if tt.failCommand != "" {
				env["RADIUS_TERRAFORM_FAIL_COMMAND"] = tt.failCommand
			}
			require.NoError(t, tf.SetEnv(env))

			var logs string
			logger := funcr.New(func(_, message string) { logs += message }, funcr.Options{})
			ctx := logr.NewContext(t.Context(), logger)
			tf.SetStdout(&tfLogWrapper{logger: logger})
			err = initAndDestroy(ctx, tf, "37s", cfg, "recipe")
			require.NotContains(t, logs, encoded.(string))
			require.NotContains(t, logs, "previous-secret")
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			var destroyed, refreshed bool
			for _, snapshot := range readBackendSnapshots(t, snapshotsPath) {
				if snapshot.Command == "apply" {
					require.Contains(t, snapshot.Args, "-refresh-only")
					require.Contains(t, snapshot.Args, "-lock-timeout=37s")
					var actual config.TerraformConfig
					require.NoError(t, json.Unmarshal(snapshot.Config, &actual))
					output := actual.Output[config.RecipeInputsOutputName].(map[string]any)
					require.Equal(t, encoded, output["value"])
					refreshed = true
					continue
				}
				if snapshot.Command != "destroy" {
					continue
				}
				destroyed = true
				var actual config.TerraformConfig
				require.NoError(t, json.Unmarshal(snapshot.Config, &actual))
				require.Equal(t, "test/module", actual.Module["recipe"]["source"])
				if tt.restored {
					require.Equal(t, inputs["context"], actual.Module["recipe"]["context"])
				} else {
					require.NotContains(t, string(snapshot.Config), "previous-secret")
				}
				require.Contains(t, snapshot.Args, "-lock-timeout=37s")
			}
			require.Equal(t, !tt.noDestroy, destroyed)
			require.Equal(t, tt.wantRefresh, refreshed)
		})
	}
}

func TestRecipeInputsTerraformLifecycle(t *testing.T) {
	terraformPath, err := exec.LookPath("terraform")
	if err != nil {
		if os.Getenv("RADIUS_REQUIRE_TERRAFORM_TESTS") == "true" {
			t.Fatal("Terraform is required but not installed")
		}
		t.Skip("Terraform is not installed")
	}
	workingDir := t.TempDir()
	moduleDir := filepath.Join(workingDir, "module")
	require.NoError(t, os.Mkdir(moduleDir, 0700))

	// The destroy provisioner fails once, so the test can retry after a partial destroy failure.
	marker := filepath.Join(t.TempDir(), "destroy-attempted")
	interpreter := `["/bin/sh", "-c"]`
	failOnce := fmt.Sprintf(`if [ -f %[1]q ]; then exit 0; fi; touch %[1]q; exit 1`, marker)
	if goruntime.GOOS == "windows" {
		interpreter = `["powershell", "-NoProfile", "-NonInteractive", "-Command"]`
		failOnce = fmt.Sprintf(`if (Test-Path -LiteralPath '%[1]s') { exit 0 }; New-Item -ItemType File -Path '%[1]s' | Out-Null; exit 1`, marker)
	}
	writeModule := func(inputName string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), fmt.Appendf(nil, `
variable "context" {
  type = any
  validation {
    condition = var.context.resource.properties.data.password.value != null
    error_message = "A secret value is required."
  }
}
variable %q {
  type = string
}
resource "terraform_data" "secret" {
  input = var.context.resource.properties.data.password.value
  provisioner "local-exec" {
    when        = destroy
    interpreter = %s
    command     = %s
  }
}
output "result" {
  value = {}
}
`, inputName, interpreter, strconv.Quote(failOnce)), 0600))
	}
	tf, err := tfexec.NewTerraform(workingDir, terraformPath)
	require.NoError(t, err)
	newConfig := func(value any, inputName string) *config.TerraformConfig {
		t.Helper()
		cfg := &config.TerraformConfig{Module: map[string]config.TFModuleConfig{
			"recipe": {
				"source": "./module",
				"context": map[string]any{
					"resource": map[string]any{
						"properties": map[string]any{
							"data": map[string]any{"password": map[string]any{"value": value}},
						},
					},
				},
				inputName: inputName + "-value",
			},
		}}
		require.NoError(t, cfg.AddRecipeInputs("recipe"))
		require.NoError(t, cfg.Save(t.Context(), workingDir))
		return cfg
	}

	writeModule("removed_input")
	newConfig(nil, "removed_input")
	_, err = initAndApply(t.Context(), tf, "37s")
	require.ErrorContains(t, err, "A secret value is required")

	newConfig("previous-secret", "removed_input")
	state, err := initAndApply(t.Context(), tf, "37s")
	require.NoError(t, err)
	require.NotContains(t, state.Values.Outputs, config.RecipeInputsOutputName)
	outputs, err := tf.Output(t.Context())
	require.NoError(t, err)
	require.True(t, outputs[config.RecipeInputsOutputName].Sensitive)

	// A new module version removes one input and adds a required input. Deletion must use the
	// current arguments and only fill the redacted secret from the saved inputs.
	writeModule("added_input")
	newConfig(nil, "added_input")
	require.ErrorContains(t, tf.Destroy(t.Context()), "A secret value is required")

	err = initAndDestroy(t.Context(), tf, "37s", newConfig(nil, "added_input"), "recipe")
	require.ErrorContains(t, err, "terraform destroy failure")
	require.FileExists(t, marker)
	outputs, err = tf.Output(t.Context())
	require.NoError(t, err)
	require.Contains(t, outputs, config.RecipeInputsOutputName, "a failed destroy must keep the saved inputs for a retry")

	require.NoError(t, initAndDestroy(t.Context(), tf, "37s", newConfig(nil, "added_input"), "recipe"))
	state, err = tf.Show(t.Context())
	require.NoError(t, err)
	require.Nil(t, state.Values)
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
			_, err = e.generateConfig(ctx, tf, tc.opts, backends.CloudBackendAuth{}, requireValidOutputMappings)
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
	_, err = e.generateConfig(t.Context(), tf, options, backends.CloudBackendAuth{}, requireValidOutputMappings)
	require.EqualError(t, err, `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["host"] -> "missing"; available module outputs: "endpoint"`)
}

func TestGenerateConfigRetainsRecipeInputs(t *testing.T) {
	for _, wrapped := range []bool{true, false} {
		t.Run(fmt.Sprintf("context_variable_%t", wrapped), func(t *testing.T) {
			workingDir := t.TempDir()
			tf, err := tfexec.NewTerraform(workingDir, os.Args[0])
			require.NoError(t, err)
			env := map[string]string{terraformGetTestHelper: "1"}
			if wrapped {
				env["RADIUS_TERRAFORM_CONTEXT_VARIABLE"] = "1"
			}
			require.NoError(t, tf.SetEnv(env))
			kubernetesClients := kubernetesclientprovider.KubernetesClientProvider{}
			kubernetesClients.SetClientGoClient(fake.NewSimpleClientset())
			e := executor{kubernetesClients: kubernetesClients}
			options := Options{
				EnvConfig: &recipes.Configuration{},
				EnvRecipe: &recipes.EnvironmentDefinition{
					Name: "test-recipe", TemplatePath: "test/module/source",
				},
				ResourceRecipe: &recipes.ResourceMetadata{
					ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/widgets/widget",
					EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/env",
					Properties: map[string]any{
						"data": map[string]any{"password": map[string]any{"value": "previous-secret"}},
					},
				},
			}
			if !wrapped {
				options.EnvRecipe.Parameters = map[string]any{
					"secret_data": "{{context.resource.properties.data}}",
				}
			}

			backend, err := e.generateConfig(t.Context(), tf, options, backends.CloudBackendAuth{}, requireValidOutputMappings)
			require.NoError(t, err)
			output := backend.config.Output[config.RecipeInputsOutputName].(map[string]any)
			require.Equal(t, true, output["sensitive"])
			require.NoError(t, backend.config.RestoreRecipeInputs("test-recipe", output["value"].(string)))
			if wrapped {
				recipeContext := backend.config.Module["test-recipe"]["context"].(map[string]any)
				resource := recipeContext["resource"].(map[string]any)
				require.Equal(t, options.ResourceRecipe.Properties, resource["properties"])
			} else {
				require.Equal(t, options.ResourceRecipe.Properties["data"], backend.config.Module["test-recipe"]["secret_data"])
			}
		})
	}
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

func Test_GetTerraformConfig_NoProviderAliasPlaceholders(t *testing.T) {
	testDir := t.TempDir()

	options := Options{
		EnvConfig: &recipes.Configuration{
			RecipeConfig: dm.RecipeConfigProperties{
				Terraform: dm.TerraformConfigProperties{
					Providers: map[string][]dm.ProviderConfigProperties{
						"azurerm": {
							{AdditionalProperties: map[string]any{"subscription_id": "sub"}},
							{AdditionalProperties: map[string]any{"alias": "custom", "subscription_id": "sub"}},
						},
					},
				},
			},
		},
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	// Placeholders are added only to retry a failed module download, so the first download
	// uses the module configuration only.
	expectedConfig := config.TerraformConfig{
		Module: map[string]config.TFModuleConfig{
			"test-recipe": {"source": "test/module/source"},
		},
	}
	tfConfig, err := getTerraformConfig(t.Context(), testDir, options)
	require.NoError(t, err)
	require.Equal(t, &expectedConfig, tfConfig)

	content, err := os.ReadFile(filepath.Join(testDir, "main.tf.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"terraform": null,
		"module": {"test-recipe": {"source": "test/module/source"}}
	}`, string(content))
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
			_, err = e.prepareExecution(t.Context(), tf, tc.opts)

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

func TestPrepareExecutionCLIConfig(t *testing.T) {
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
			_, err = e.prepareExecution(t.Context(), tf, tc.opts)
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
