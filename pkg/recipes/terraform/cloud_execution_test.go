/*
Copyright 2026 The Radius Authors.

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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

const terraformBackendSnapshots = "RADIUS_TERRAFORM_BACKEND_SNAPSHOTS"

type backendExecutionSnapshot struct {
	Command     string
	Args        []string
	Environment map[string]string
	Config      json.RawMessage
	CLIConfig   string
}

// recordBackendTestExecution runs inside a real subprocess launched by terraform-exec.
func recordBackendTestExecution() error {
	record := backendExecutionSnapshot{Command: os.Args[1], Args: os.Args[2:], Environment: map[string]string{}}
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"ARM_CLIENT_ID", "ARM_CLIENT_SECRET", "ARM_TENANT_ID", "ARM_USE_OIDC", "ARM_OIDC_TOKEN_FILE_PATH",
		"ARM_CLIENT_ID_FILE_PATH", "ARM_CLIENT_SECRET_FILE_PATH", "ARM_CLIENT_CERTIFICATE_PATH",
		"ARM_ACCESS_KEY", "ARM_SAS_TOKEN", "TEST_USER_ENV", "TEST_SECRET_ENV", envTFCLIConfigFile,
	} {
		if value, ok := os.LookupEnv(key); ok {
			record.Environment[key] = value
		}
	}
	var err error
	record.Config, err = os.ReadFile("main.tf.json")
	if err != nil {
		return err
	}
	if path := os.Getenv(envTFCLIConfigFile); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		record.CLIConfig = string(content)
	}
	file, err := os.OpenFile(os.Getenv(terraformBackendSnapshots), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(record); err != nil {
		return err
	}
	if record.Command == os.Getenv("RADIUS_TERRAFORM_FAIL_COMMAND") {
		return fmt.Errorf("simulated Terraform command failure")
	}
	return nil
}

func installBackendTestTerraform(t *testing.T) string {
	t.Helper()
	// Snapshots must never capture credentials or CLI configuration from the developer's host.
	t.Setenv(envTFCLIConfigFile, "")
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"ARM_CLIENT_ID", "ARM_CLIENT_SECRET", "ARM_TENANT_ID", "ARM_USE_OIDC", "ARM_OIDC_TOKEN_FILE_PATH",
		"ARM_CLIENT_ID_FILE_PATH", "ARM_CLIENT_SECRET_FILE_PATH", "ARM_CLIENT_CERTIFICATE_PATH",
		"ARM_ACCESS_KEY", "ARM_SAS_TOKEN",
	} {
		t.Setenv(key, "host-stale")
	}
	globalDir := t.TempDir()
	t.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalDir)
	t.Setenv(terraformGetTestHelper, "1")
	require.NoError(t, os.Symlink(os.Args[0], filepath.Join(globalDir, "terraform")))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, ".terraform-ready"), nil, 0600))
	globalTerraformMutex.Lock()
	previous := globalTerraformReady
	globalTerraformReady = true
	globalTerraformMutex.Unlock()
	t.Cleanup(func() {
		globalTerraformMutex.Lock()
		globalTerraformReady = previous
		globalTerraformMutex.Unlock()
	})
	snapshots := filepath.Join(t.TempDir(), "executions.jsonl")
	t.Setenv(terraformBackendSnapshots, snapshots)
	return snapshots
}

func readBackendSnapshots(t *testing.T, path string) []backendExecutionSnapshot {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	var snapshots []backendExecutionSnapshot
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var snapshot backendExecutionSnapshot
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &snapshot))
		snapshots = append(snapshots, snapshot)
	}
	require.NoError(t, scanner.Err())
	return snapshots
}

func cloudExecutionOptions(t *testing.T, cloud string) Options {
	t.Helper()
	backend := &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}
	if cloud == "azurerm" {
		backend = &datamodel.TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"}
	}
	return Options{
		RootDir: t.TempDir(), StateLockTimeout: "37s",
		EnvConfig: &recipes.Configuration{TerraformBackend: backend, RecipeConfig: datamodel.RecipeConfigProperties{
			Env: datamodel.EnvironmentVariables{AdditionalProperties: map[string]string{
				"TEST_USER_ENV": "user-value", "AWS_ACCESS_KEY_ID": "user-access", "ARM_CLIENT_SECRET": "user-secret",
			}},
			EnvSecrets: map[string]datamodel.SecretReference{"TEST_SECRET_ENV": {Source: "secret-store", Key: "env"}},
			Terraform: datamodel.TerraformConfigProperties{
				Credentials: map[string]datamodel.TerraformCredentialConfig{"registry.example.com": {Secret: "secret-store"}},
				ProviderInstallation: &datamodel.TerraformProviderInstallation{
					NetworkMirror: &datamodel.TerraformProviderMirror{URL: "https://mirror.example.com/"},
				},
			},
		}},
		Secrets:   map[string]recipes.SecretData{"secret-store": {Data: map[string]string{"env": "env-secret", "token": "registry-token"}}},
		EnvRecipe: &recipes.EnvironmentDefinition{Name: "test-recipe", TemplatePath: "test/module/source", ResourceType: "Test.Resources/widgets"},
		ResourceRecipe: &recipes.ResourceMetadata{
			Name:          "widget",
			ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/widgets/widget",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
		},
	}
}

func TestCloudDeployUpdateDeleteProcessEnvironment(t *testing.T) {
	for _, cloud := range []string{"s3", "azurerm"} {
		for _, federated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/federated=%v", cloud, federated), func(t *testing.T) {
				snapshotsPath := installBackendTestTerraform(t)
				t.Setenv("AWS_ACCESS_KEY_ID", "host-access")
				t.Setenv("AWS_SESSION_TOKEN", "host-session")
				t.Setenv("ARM_CLIENT_SECRET", "host-secret")
				t.Setenv("ARM_ACCESS_KEY", "host-storage-key")
				aws := &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(federated)}
				azure := &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(federated)}
				kube := fake.NewSimpleClientset()
				kubeProvider := kubernetesclientprovider.KubernetesClientProvider{}
				kubeProvider.SetClientGoClient(kube)
				e := executor{awsCredentials: aws, azureCredentials: azure, kubernetesClients: kubeProvider}
				options := cloudExecutionOptions(t, cloud)
				var logMu sync.Mutex
				var logs strings.Builder
				ctx := logr.NewContext(t.Context(), funcr.New(func(prefix, args string) {
					logMu.Lock()
					defer logMu.Unlock()
					logs.WriteString(prefix + args)
				}, funcr.Options{}))
				for range 2 {
					state, err := e.Deploy(ctx, options)
					require.NoError(t, err)
					require.Equal(t, "declared", state.Values.Outputs["endpoint"].Value)
				}
				require.NoError(t, e.Delete(ctx, options))
				require.Empty(t, kube.Actions(), "cloud executions must not probe or delete Kubernetes state secrets")
				if cloud == "s3" {
					require.Len(t, aws.names, 3)
					require.Empty(t, azure.names)
				} else {
					require.Len(t, azure.names, 3)
					require.Empty(t, aws.names)
				}
				snapshots := readBackendSnapshots(t, snapshotsPath)
				var stateKey string
				var commands []string
				for _, snapshot := range snapshots {
					commands = append(commands, snapshot.Command)
					// terraform-exec's version probe deliberately uses the host environment.
					if snapshot.Command == "version" {
						continue
					}
					env := snapshot.Environment
					require.Equal(t, "user-value", env["TEST_USER_ENV"], snapshot.Command)
					require.Equal(t, "env-secret", env["TEST_SECRET_ENV"], snapshot.Command)
					require.Contains(t, snapshot.CLIConfig, "registry-token", "registry config is present even during terraform get")
					require.Contains(t, snapshot.CLIConfig, "https://mirror.example.com/")
					if cloud == "s3" {
						require.NotContains(t, env, "AWS_SESSION_TOKEN")
						if federated {
							require.NotContains(t, env, "AWS_ACCESS_KEY_ID")
							require.Equal(t, awsBackendTokenFile, env["AWS_WEB_IDENTITY_TOKEN_FILE"])
						} else {
							require.Equal(t, "registered-access", env["AWS_ACCESS_KEY_ID"])
							require.Equal(t, "registered-secret", env["AWS_SECRET_ACCESS_KEY"])
						}
					} else {
						require.Equal(t, "registered-client", env["ARM_CLIENT_ID"])
						require.NotContains(t, env, "ARM_ACCESS_KEY")
						if federated {
							require.NotContains(t, env, "ARM_CLIENT_SECRET")
							require.Equal(t, azureBackendTokenFile, env["ARM_OIDC_TOKEN_FILE_PATH"])
						} else {
							require.Equal(t, "registered-secret", env["ARM_CLIENT_SECRET"])
						}
					}
					if snapshot.Command == "get" {
						continue
					}
					var config map[string]any
					require.NoError(t, json.Unmarshal(snapshot.Config, &config))
					backends := config["terraform"].(map[string]any)["backend"].(map[string]any)
					require.Len(t, backends, 1)
					backend := backends[cloud].(map[string]any)
					if stateKey == "" {
						stateKey = backend["key"].(string)
					}
					require.Equal(t, stateKey, backend["key"])
					require.NotContains(t, string(snapshot.Config), "registered-secret")
					require.NotContains(t, string(snapshot.Config), "registry-token")
					if snapshot.Command == "apply" || snapshot.Command == "destroy" {
						require.Contains(t, snapshot.Args, "-lock-timeout=37s")
					}
				}
				// No recipe providers are required by the helper's module, yet backend credentials reach init.
				require.Equal(t, 3, countCommand(commands, "get"))
				require.Equal(t, 3, countCommand(commands, "init"))
				require.Equal(t, 2, countCommand(commands, "apply"))
				require.Equal(t, 1, countCommand(commands, "destroy"))
				require.Less(t, indexCommand(commands, "get"), indexCommand(commands, "init"))
				require.NotContains(t, logs.String(), "registered-secret")
				require.NotContains(t, logs.String(), "registry-token")
				require.Equal(t, "host-access", os.Getenv("AWS_ACCESS_KEY_ID"))
				require.Equal(t, "host-session", os.Getenv("AWS_SESSION_TOKEN"))
				require.Equal(t, "host-secret", os.Getenv("ARM_CLIENT_SECRET"))
			})
		}
	}
}

func TestCloudAzureStaleFilePathSelectors(t *testing.T) {
	for _, federated := range []bool{false, true} {
		for _, source := range []string{"inherited", "settings"} {
			t.Run(fmt.Sprintf("%s/federated=%v", source, federated), func(t *testing.T) {
				snapshotsPath := installBackendTestTerraform(t)
				options := cloudExecutionOptions(t, "azurerm")
				stalePath := filepath.Join(t.TempDir(), "stale-credential")
				require.NoError(t, os.WriteFile(stalePath, []byte("unregistered-value"), 0600))
				selectors := []string{
					"ARM_CLIENT_ID_FILE_PATH", "ARM_CLIENT_SECRET_FILE_PATH",
					"ARM_CLIENT_CERTIFICATE_PATH", "ARM_OIDC_TOKEN_FILE_PATH",
				}
				for _, key := range selectors {
					t.Setenv(key, stalePath)
					if source == "settings" {
						options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = stalePath
						t.Setenv(key, "")
					}
				}
				azure := &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(federated)}
				e := executor{azureCredentials: azure}
				_, err := e.Deploy(t.Context(), options)
				require.NoError(t, err)
				var commands []string
				for _, snapshot := range readBackendSnapshots(t, snapshotsPath) {
					if snapshot.Command == "version" {
						continue
					}
					commands = append(commands, snapshot.Command)
					env := snapshot.Environment
					require.Equal(t, "registered-client", env["ARM_CLIENT_ID"])
					require.Equal(t, "registered-tenant", env["ARM_TENANT_ID"])
					for _, key := range selectors {
						if key == "ARM_OIDC_TOKEN_FILE_PATH" && federated {
							require.Equal(t, azureBackendTokenFile, env[key])
						} else {
							require.NotContains(t, env, key, snapshot.Command)
						}
					}
					if federated {
						require.Equal(t, "true", env["ARM_USE_OIDC"])
						require.NotContains(t, env, "ARM_CLIENT_SECRET")
					} else {
						require.Equal(t, "false", env["ARM_USE_OIDC"])
						require.Equal(t, "registered-secret", env["ARM_CLIENT_SECRET"])
					}
				}
				require.Contains(t, commands, "get")
				require.Contains(t, commands, "init")
				require.Contains(t, commands, "apply")
			})
		}
	}
}

func countCommand(commands []string, command string) int {
	n := 0
	for _, c := range commands {
		if c == command {
			n++
		}
	}
	return n
}

func indexCommand(commands []string, command string) int {
	for i, c := range commands {
		if c == command {
			return i
		}
	}
	return -1
}

func TestCloudExecutionFailureDoesNotFallback(t *testing.T) {
	for _, operation := range []string{"deploy", "delete"} {
		for _, failure := range []string{"credentials", "init", "apply", "destroy"} {
			if operation == "deploy" && failure == "destroy" || operation == "delete" && failure == "apply" {
				continue
			}

			t.Run(operation+"/"+failure, func(t *testing.T) {
				path := installBackendTestTerraform(t)
				t.Setenv("RADIUS_TERRAFORM_FAIL_COMMAND", failure)
				aws := &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(false)}
				if failure == "credentials" {
					aws.value = nil
				}
				e := executor{awsCredentials: aws}
				options := cloudExecutionOptions(t, "s3")
				var err error
				if operation == "deploy" {
					_, err = e.Deploy(t.Context(), options)
				} else {
					err = e.Delete(t.Context(), options)
				}
				require.Error(t, err)
				if failure == "credentials" {
					require.Contains(t, err.Error(), "requires registered default AWS credentials")
					_, err = os.Stat(path)
					require.True(t, os.IsNotExist(err), "credentials must fail before fetching modules")
					return
				}
				require.Contains(t, err.Error(), "terraform "+failure+" failure")
				snapshots := readBackendSnapshots(t, path)
				require.Equal(t, failure, snapshots[len(snapshots)-1].Command)
			})
		}
	}
}

func TestCloudBackendPreservesExplicitProviderAuthentication(t *testing.T) {
	for _, cloud := range []string{"s3", "azurerm"} {
		t.Run(cloud, func(t *testing.T) {
			path := installBackendTestTerraform(t)
			options := cloudExecutionOptions(t, cloud)
			provider := "aws"
			explicit := map[string]any{"access_key": "provider-access", "secret_key": "provider-secret", "region": "us-east-1"}
			if cloud == "azurerm" {
				provider = "azurerm"
				explicit = map[string]any{
					"client_id": "provider-client", "client_secret": "provider-secret", "tenant_id": "provider-tenant",
					"subscription_id": "provider-subscription", "use_oidc": false, "use_cli": false, "features": map[string]any{},
				}
			}
			t.Setenv("RADIUS_TERRAFORM_REQUIRED_PROVIDER", provider)
			options.EnvConfig.RecipeConfig.Terraform.Providers = map[string][]datamodel.ProviderConfigProperties{
				provider: {{AdditionalProperties: explicit}},
			}
			e := executor{
				awsCredentials:   &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(true)},
				azureCredentials: &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(true)},
			}
			_, err := e.Deploy(t.Context(), options)
			require.NoError(t, err)
			var checked bool
			for _, snapshot := range readBackendSnapshots(t, path) {
				if snapshot.Command != "init" {
					continue
				}
				checked = true
				var config map[string]any
				require.NoError(t, json.Unmarshal(snapshot.Config, &config))
				actualProvider := config["provider"].(map[string]any)[provider].([]any)[0]
				require.Equal(t, explicit, actualProvider, "backend authentication must not rewrite explicit provider configuration")
				backendJSON, err := json.Marshal(config["terraform"].(map[string]any)["backend"])
				require.NoError(t, err)
				require.NotContains(t, string(backendJSON), "provider-secret")
				require.NotContains(t, string(backendJSON), "registered")
				if cloud == "s3" {
					require.Equal(t, awsBackendTokenFile, snapshot.Environment["AWS_WEB_IDENTITY_TOKEN_FILE"])
				} else {
					require.Equal(t, azureBackendTokenFile, snapshot.Environment["ARM_OIDC_TOKEN_FILE_PATH"])
				}
			}
			require.True(t, checked, "must reach Terraform init")
		})
	}
}

func TestCloudMetadataDoesNotFetchBackendCredentials(t *testing.T) {
	path := installBackendTestTerraform(t)
	aws := &backendCredentialStub[credentials.AWSCredential]{}
	azure := &backendCredentialStub[credentials.AzureCredential]{}
	e := executor{awsCredentials: aws, azureCredentials: azure}
	metadata, err := e.GetRecipeMetadata(t.Context(), cloudExecutionOptions(t, "s3"))
	require.NoError(t, err)
	require.Contains(t, metadata, "parameters")
	require.Empty(t, aws.names)
	require.Empty(t, azure.names)
	snapshots := readBackendSnapshots(t, path)
	require.Len(t, snapshots, 1)
	require.Equal(t, "get", snapshots[0].Command)
}
