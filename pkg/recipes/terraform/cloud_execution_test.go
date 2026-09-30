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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
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
		"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3", "AWS_S3_ENDPOINT", "AWS_ENDPOINT_URL_STS", "AWS_STS_ENDPOINT",
		"ARM_CLIENT_ID", "ARM_CLIENT_SECRET", "ARM_TENANT_ID", "ARM_USE_OIDC", "ARM_OIDC_TOKEN_FILE_PATH",
		"ARM_CLIENT_ID_FILE_PATH", "ARM_CLIENT_SECRET_FILE_PATH", "ARM_CLIENT_CERTIFICATE_PATH",
		"ARM_CLIENT_CERTIFICATE", "ARM_CLIENT_CERTIFICATE_PASSWORD", "ARM_OIDC_TOKEN",
		"ARM_ACCESS_KEY", "ARM_SAS_TOKEN", "ARM_SUBSCRIPTION_ID", "ARM_METADATA_HOSTNAME", "ARM_METADATA_HOST",
		"TEST_USER_ENV", "TEST_SECRET_ENV", envTFCLIConfigFile,
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
	for _, key := range awsBackendTestEndpointVariables {
		t.Setenv(key, "")
	}
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

// clearAzureConflictingAuthentication resets the Azure variables that Terraform resolves ahead of a
// workload identity rendered into the backend block. installBackendTestTerraform seeds several of
// them to simulate a dirty host, and an azurerm backend using workload identity rejects them, so
// tests that exercise a successful execution must start without them.
func clearAzureConflictingAuthentication(t *testing.T) {
	t.Helper()
	for _, key := range azureBackendConflictingAuthVariables {
		t.Setenv(key, "")
	}
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
	backend := &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: "radius"}
	if cloud == "azurerm" {
		backend = &datamodel.TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius", KeyPrefix: "radius"}
	}
	return Options{
		RootDir: t.TempDir(), StateLockTimeout: "37s",
		EnvConfig: &recipes.Configuration{TerraformBackend: backend, RecipeConfig: datamodel.RecipeConfigProperties{
			Env: datamodel.EnvironmentVariables{AdditionalProperties: map[string]string{
				"TEST_USER_ENV": "user-value", "AWS_ACCESS_KEY_ID": "user-access", "ARM_SUBSCRIPTION_ID": "user-subscription",
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

// backendSessionName derives the expected AWS session name from a rendered state key, so that state
// access in CloudTrail can be traced back to the Radius resource that caused it.
func noStateCleanup(_ context.Context, _ *datamodel.TerraformBackend, _ backends.CloudBackendAuth, _ string) error {
	return nil
}

func backendSessionName(stateKey string) string {
	return "radius-tf-backend-" + strings.TrimSuffix(path.Base(stateKey), ".tfstate")
}

func TestCloudDeployUpdateDeleteProcessEnvironment(t *testing.T) {
	for _, cloud := range []string{"s3", "azurerm"} {
		for _, federated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/federated=%v", cloud, federated), func(t *testing.T) {
				snapshotsPath := installBackendTestTerraform(t)
				// An azurerm backend using workload identity rejects conflicting Azure auth, so
				// seed it only where it is legitimately carried through to the module.
				azureIdentity := cloud == "azurerm" && federated
				t.Setenv("AWS_ACCESS_KEY_ID", "host-access")
				t.Setenv("AWS_SESSION_TOKEN", "host-session")
				if azureIdentity {
					clearAzureConflictingAuthentication(t)
				} else {
					t.Setenv("ARM_CLIENT_SECRET", "host-secret")
					t.Setenv("ARM_ACCESS_KEY", "host-storage-key")
				}
				aws := &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(federated)}
				azure := &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(federated)}
				kube := fake.NewSimpleClientset()
				kubeProvider := kubernetesclientprovider.KubernetesClientProvider{}
				kubeProvider.SetClientGoClient(kube)
				var cleaned []string
				e := executor{awsCredentials: aws, azureCredentials: azure, kubernetesClients: kubeProvider,
					deleteStateObject: func(_ context.Context, settings *datamodel.TerraformBackend, _ backends.CloudBackendAuth, key string) error {
						cleaned = append(cleaned, settings.Type+":"+key)
						return nil
					}}
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
				// Destroy leaves an empty state object behind; deleting the resource must remove it.
				require.Len(t, cleaned, 1)
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
					if federated {
						// Identity credentials are rendered into the backend block, so the
						// environment the recipe's providers see is left exactly as configured.
						require.Equal(t, "user-access", env["AWS_ACCESS_KEY_ID"])
						require.Equal(t, "host-session", env["AWS_SESSION_TOKEN"])
						require.Equal(t, "user-subscription", env["ARM_SUBSCRIPTION_ID"])
						if cloud == "s3" {
							// Azure variables cannot reach an S3 backend, so they survive untouched.
							require.Equal(t, "host-secret", env["ARM_CLIENT_SECRET"])
							require.Equal(t, "host-storage-key", env["ARM_ACCESS_KEY"])
						} else {
							// Terraform would resolve these ahead of the rendered workload
							// identity, so execution is rejected while any of them is set.
							require.Empty(t, env["ARM_CLIENT_SECRET"])
							require.Empty(t, env["ARM_ACCESS_KEY"])
						}
					} else if cloud == "s3" {
						require.NotContains(t, env, "AWS_SESSION_TOKEN")
						require.NotContains(t, env, "AWS_WEB_IDENTITY_TOKEN_FILE")
						require.Equal(t, "registered-access", env["AWS_ACCESS_KEY_ID"])
						require.Equal(t, "registered-secret", env["AWS_SECRET_ACCESS_KEY"])
					} else {
						require.Equal(t, "registered-client", env["ARM_CLIENT_ID"])
						require.Equal(t, "registered-secret", env["ARM_CLIENT_SECRET"])
						require.NotContains(t, env, "ARM_ACCESS_KEY")
						require.NotContains(t, env, "ARM_OIDC_TOKEN_FILE_PATH")
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
					if federated && cloud == "s3" {
						assume := backend["assume_role_with_web_identity"].(map[string]any)
						require.Equal(t, "arn:aws:iam::123456789012:role/radius-state", assume["role_arn"])
						require.Equal(t, providers.AWSIRSATokenFilePath, assume["web_identity_token_file"])
						// The session name must attribute state access to a Radius resource in CloudTrail.
						require.Equal(t, backendSessionName(stateKey), assume["session_name"])
						require.LessOrEqual(t, len(assume["session_name"].(string)), 64)
					} else if federated {
						require.Equal(t, true, backend["use_oidc"])
						require.Equal(t, "registered-client", backend["client_id"])
						require.Equal(t, "registered-tenant", backend["tenant_id"])
						require.Equal(t, providers.AzureOIDCTokenFilePath, backend["oidc_token_file_path"])
					} else {
						require.NotContains(t, backend, "assume_role_with_web_identity")
						require.NotContains(t, backend, "use_oidc")
					}
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
				if !azureIdentity {
					require.Equal(t, "host-secret", os.Getenv("ARM_CLIENT_SECRET"))
				}
			})
		}
	}
}

// Stale ARM file-path selectors must not survive into a ServicePrincipal backend execution, where
// they would otherwise outrank the registered credential.
func TestCloudAzureStaleFilePathSelectors(t *testing.T) {
	for _, source := range []string{"inherited", "settings"} {
		t.Run(source, func(t *testing.T) {
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
			azure := &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(false)}
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
					require.NotContains(t, env, key, snapshot.Command)
				}
				require.Equal(t, "false", env["ARM_USE_OIDC"])
				require.Equal(t, "registered-secret", env["ARM_CLIENT_SECRET"])
			}
			require.Contains(t, commands, "get")
			require.Contains(t, commands, "init")
			require.Contains(t, commands, "apply")
		})
	}
}

// Terraform resolves several environment variables ahead of a workload identity rendered into the
// backend block, so each must be rejected before execution rather than silently authenticating
// state access as something other than the registered credential.
func TestAzureBackendRejectsConflictingAuthentication(t *testing.T) {
	for _, source := range []string{"inherited", "settings", "secret"} {
		for _, key := range azureBackendConflictingAuthVariables {
			for _, operation := range []string{"deploy", "delete"} {
				t.Run(fmt.Sprintf("%s/%s/%s", operation, source, key), func(t *testing.T) {
					snapshotsPath := installBackendTestTerraform(t)
					clearAzureConflictingAuthentication(t)
					options := cloudExecutionOptions(t, "azurerm")
					const conflicting = "private-value"
					switch source {
					case "inherited":
						t.Setenv(key, conflicting)
					case "settings":
						options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = conflicting
					case "secret":
						options.EnvConfig.RecipeConfig.EnvSecrets[key] = datamodel.SecretReference{Source: "secret-store", Key: "conflict"}
						options.Secrets["secret-store"].Data["conflict"] = conflicting
					}
					before := maps.Clone(options.EnvConfig.RecipeConfig.Env.AdditionalProperties)
					hostValue := os.Getenv(key)
					e := executor{azureCredentials: &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(true)}, deleteStateObject: noStateCleanup}
					var err error
					if operation == "deploy" {
						_, err = e.Deploy(t.Context(), options)
					} else {
						err = e.Delete(t.Context(), options)
					}
					require.ErrorContains(t, err, key)
					require.Contains(t, err.Error(), "registered Azure WorkloadIdentity credentials")
					require.NotContains(t, err.Error(), conflicting)
					require.NoFileExists(t, snapshotsPath, "must reject before any Terraform subprocess runs")
					require.Equal(t, before, options.EnvConfig.RecipeConfig.Env.AdditionalProperties)
					require.Equal(t, hostValue, os.Getenv(key))
				})
			}
		}
	}
}

// A ServicePrincipal backend delivers its credential through the environment and scrubs the same
// variables, so unlike workload identity it must keep working when they are present.
func TestAzureServicePrincipalBackendScrubsConflictingAuthentication(t *testing.T) {
	for _, key := range azureBackendConflictingAuthVariables {
		t.Run(key, func(t *testing.T) {
			snapshotsPath := installBackendTestTerraform(t)
			options := cloudExecutionOptions(t, "azurerm")
			options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = "user-value"
			e := executor{azureCredentials: &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(false)}, deleteStateObject: noStateCleanup}
			_, err := e.Deploy(t.Context(), options)
			require.NoError(t, err)
			for _, snapshot := range readBackendSnapshots(t, snapshotsPath) {
				if snapshot.Command == "version" {
					continue
				}
				require.Equal(t, "registered-client", snapshot.Environment["ARM_CLIENT_ID"])
				if key == "ARM_CLIENT_SECRET" {
					require.Equal(t, "registered-secret", snapshot.Environment[key])
					continue
				}
				require.Empty(t, snapshot.Environment[key], "conflicting selector must not survive")
			}
		})
	}
}

// Metadata host overrides redirect Azure endpoint discovery for state traffic and for the token
// exchange, so both credential modes must reject them.
func TestAzureBackendRejectsMetadataHostOverride(t *testing.T) {
	for _, federated := range []bool{false, true} {
		for _, key := range azureBackendMetadataVariables {
			for _, operation := range []string{"deploy", "delete"} {
				t.Run(fmt.Sprintf("%s/%s/federated=%v", operation, key, federated), func(t *testing.T) {
					snapshotsPath := installBackendTestTerraform(t)
					clearAzureConflictingAuthentication(t)
					options := cloudExecutionOptions(t, "azurerm")
					t.Setenv(key, "metadata.private.example.com")
					e := executor{azureCredentials: &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(federated)}, deleteStateObject: noStateCleanup}
					var err error
					if operation == "deploy" {
						_, err = e.Deploy(t.Context(), options)
					} else {
						err = e.Delete(t.Context(), options)
					}
					require.ErrorContains(t, err, key)
					require.Contains(t, err.Error(), "does not support endpoint override")
					require.NoFileExists(t, snapshotsPath, "must reject before any Terraform subprocess runs")
				})
			}
		}
	}
}

func TestCloudS3RejectsEndpointOverridesBeforeExecution(t *testing.T) {
	for _, federated := range []bool{false, true} {
		for _, source := range []string{"inherited", "settings", "secret"} {
			for _, key := range awsBackendTestEndpointVariables {
				for _, operation := range []string{"deploy", "delete"} {
					t.Run(fmt.Sprintf("%s/%s/%s/federated=%v", operation, source, key, federated), func(t *testing.T) {
						path := installBackendTestTerraform(t)
						options := cloudExecutionOptions(t, "s3")
						const endpoint = "https://endpoint.example.com/private-value"
						switch source {
						case "inherited":
							t.Setenv(key, endpoint)
						case "settings":
							options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = endpoint
						case "secret":
							options.EnvConfig.RecipeConfig.EnvSecrets[key] = datamodel.SecretReference{Source: "secret-store", Key: "endpoint"}
							options.Secrets["secret-store"].Data["endpoint"] = endpoint
						}
						before := maps.Clone(options.EnvConfig.RecipeConfig.Env.AdditionalProperties)
						hostValue := os.Getenv(key)
						e := executor{awsCredentials: &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(federated)}, deleteStateObject: noStateCleanup}
						var err error
						if operation == "deploy" {
							_, err = e.Deploy(t.Context(), options)
						} else {
							err = e.Delete(t.Context(), options)
						}
						require.ErrorContains(t, err, key)
						require.Contains(t, err.Error(), "s3 backend does not support endpoint override")
						require.NotContains(t, err.Error(), "private-value")
						require.NoFileExists(t, path, "must reject before any Terraform subprocess runs")
						require.Equal(t, before, options.EnvConfig.RecipeConfig.Env.AdditionalProperties)
						require.Equal(t, hostValue, os.Getenv(key))
					})
				}
			}
		}
	}
}

func TestCloudS3EndpointOverridePrecedence(t *testing.T) {
	for _, federated := range []bool{false, true} {
		for _, settingsValue := range []string{"", "https://settings.example.com"} {
			t.Run(fmt.Sprintf("federated=%v/settings=%q", federated, settingsValue), func(t *testing.T) {
				path := installBackendTestTerraform(t)
				options := cloudExecutionOptions(t, "s3")
				for _, key := range awsBackendTestEndpointVariables {
					t.Setenv(key, "https://inherited.example.com")
					options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = settingsValue
				}
				e := executor{awsCredentials: &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(federated)}, deleteStateObject: noStateCleanup}
				_, err := e.Deploy(t.Context(), options)
				if settingsValue != "" {
					require.ErrorContains(t, err, "s3 backend does not support endpoint override")
					require.NoFileExists(t, path)
					return
				}
				require.NoError(t, err)
				require.NoError(t, e.Delete(t.Context(), options))
				var commands []string
				for _, snapshot := range readBackendSnapshots(t, path) {
					// terraform-exec's version probe uses the host environment, not the execution environment.
					if snapshot.Command == "version" {
						continue
					}
					commands = append(commands, snapshot.Command)
					for _, key := range awsBackendTestEndpointVariables {
						require.Contains(t, snapshot.Environment, key)
						require.Empty(t, snapshot.Environment[key], snapshot.Command)
					}
					if federated {
						// IRSA delivers nothing through the environment; the role is rendered into
						// the backend block instead. Asserting the host values survive verbatim is
						// stronger than inequality, which would pass on an unset variable.
						require.Equal(t, "host-stale", snapshot.Environment["AWS_WEB_IDENTITY_TOKEN_FILE"], snapshot.Command)
						require.Equal(t, "host-stale", snapshot.Environment["AWS_ROLE_ARN"], snapshot.Command)
						if snapshot.Command != "get" {
							require.Contains(t, string(snapshot.Config), providers.AWSIRSATokenFilePath)
						}
					} else {
						require.Equal(t, "registered-secret", snapshot.Environment["AWS_SECRET_ACCESS_KEY"])
					}
				}
				for _, command := range []string{"get", "init", "apply", "destroy"} {
					require.Contains(t, commands, command)
				}
				for _, key := range awsBackendTestEndpointVariables {
					require.Equal(t, "https://inherited.example.com", os.Getenv(key))
					require.Empty(t, options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key])
				}
			})
		}
	}
}

func TestNonS3BackendPreservesAWSEndpointOverrides(t *testing.T) {
	for _, cloud := range []string{"kubernetes", "azurerm"} {
		for _, source := range []string{"inherited", "settings"} {
			t.Run(cloud+"/"+source, func(t *testing.T) {
				path := installBackendTestTerraform(t)
				options := cloudExecutionOptions(t, cloud)
				if cloud == "kubernetes" {
					options.EnvConfig.TerraformBackend = nil
				}
				for _, key := range awsBackendTestEndpointVariables {
					if source == "inherited" {
						t.Setenv(key, "https://provider.example.com")
					} else {
						options.EnvConfig.RecipeConfig.Env.AdditionalProperties[key] = "https://provider.example.com"
					}
				}
				aws := &backendCredentialStub[credentials.AWSCredential]{}
				e := executor{
					awsCredentials:   aws,
					azureCredentials: &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(false)},
				}
				tf, err := tfexec.NewTerraform(options.RootDir, os.Args[0])
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(options.RootDir, "main.tf.json"), []byte("{}"), 0600))
				_, err = e.prepareExecution(t.Context(), tf, options)
				require.NoError(t, err)
				require.NoError(t, tf.Get(t.Context()))
				require.Empty(t, aws.names)
				snapshots := readBackendSnapshots(t, path)
				require.Len(t, snapshots, 1)
				require.Equal(t, "get", snapshots[0].Command)
				for _, key := range awsBackendTestEndpointVariables {
					require.Equal(t, "https://provider.example.com", snapshots[0].Environment[key])
				}
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
				e := executor{awsCredentials: aws, deleteStateObject: noStateCleanup}
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
				// The backend uses workload identity here, which rejects conflicting Azure auth
				// in the environment. Provider credentials stay in the provider block.
				clearAzureConflictingAuthentication(t)
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
				require.NotContains(t, string(backendJSON), "registered-secret")
				// Identity-based backend authentication lives in the backend block, so the
				// environment shared with the module's providers stays untouched.
				require.NotEqual(t, providers.AWSIRSATokenFilePath, snapshot.Environment["AWS_WEB_IDENTITY_TOKEN_FILE"])
				require.NotEqual(t, providers.AzureOIDCTokenFilePath, snapshot.Environment["ARM_OIDC_TOKEN_FILE_PATH"])
				if cloud == "s3" {
					require.Contains(t, string(backendJSON), providers.AWSIRSATokenFilePath)
				} else {
					require.Contains(t, string(backendJSON), providers.AzureOIDCTokenFilePath)
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

// State cleanup runs after the resources are already destroyed, so a failure must be logged rather
// than left to fail the Radius delete and make the resource undeletable.
func TestCloudStateCleanupIsBestEffort(t *testing.T) {
	for _, cleanupErr := range []error{nil, errors.New("access denied to state object")} {
		t.Run(fmt.Sprintf("error=%v", cleanupErr != nil), func(t *testing.T) {
			installBackendTestTerraform(t)
			options := cloudExecutionOptions(t, "s3")
			var gotType, gotKey string
			e := executor{
				awsCredentials: &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(true)},
				deleteStateObject: func(_ context.Context, settings *datamodel.TerraformBackend, _ backends.CloudBackendAuth, key string) error {
					gotType, gotKey = settings.Type, key
					return cleanupErr
				},
			}
			var logs strings.Builder
			ctx := logr.NewContext(t.Context(), funcr.New(func(prefix, args string) {
				logs.WriteString(prefix + args)
			}, funcr.Options{}))

			require.NoError(t, e.Delete(ctx, options))
			require.Equal(t, "s3", gotType)
			require.Regexp(t, `^radius/[0-9a-f]{40}\.tfstate$`, gotKey)
			// The state key is the only way an operator can map a leftover object back to a resource.
			require.Contains(t, logs.String(), gotKey)
			if cleanupErr != nil {
				require.Contains(t, logs.String(), "access denied to state object")
			}
		})
	}
}

// A concurrent writer may replace the state between the end of destroy and cleanup. The conditional
// delete must leave that state alone, and say so distinctly from a cleanup failure so operators do
// not go hunting for an object that is deliberately still there.
func TestCloudStateCleanupSkipsModifiedState(t *testing.T) {
	installBackendTestTerraform(t)
	options := cloudExecutionOptions(t, "s3")
	var gotKey string
	e := executor{
		awsCredentials: &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(true)},
		deleteStateObject: func(_ context.Context, _ *datamodel.TerraformBackend, _ backends.CloudBackendAuth, key string) error {
			gotKey = key
			return fmt.Errorf("conditional delete rejected: %w", errStateModifiedDuringCleanup)
		},
	}
	var logs strings.Builder
	ctx := logr.NewContext(t.Context(), funcr.New(func(prefix, args string) {
		logs.WriteString(prefix + args)
	}, funcr.Options{}))

	require.NoError(t, e.Delete(ctx, options))
	require.Contains(t, logs.String(), gotKey)
	require.Contains(t, logs.String(), "changed since destroy completed")
	require.NotContains(t, logs.String(), "can be removed manually")
}

// The S3 error classifiers decide whether cleanup deletes, skips, or reports a failure, so a
// misclassification would either delete a concurrent writer's state or hide a real error.
func TestS3CleanupErrorClassification(t *testing.T) {
	responseErr := func(status int) error {
		return &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}}}
	}

	for _, tc := range []struct {
		name         string
		err          error
		notFound     bool
		precondition bool
	}{
		{name: "no such key", err: &smithy.GenericAPIError{Code: "NoSuchKey"}, notFound: true},
		{name: "head not found code", err: &smithy.GenericAPIError{Code: "NotFound"}, notFound: true},
		{name: "head not found status", err: responseErr(http.StatusNotFound), notFound: true},
		{name: "precondition code", err: &smithy.GenericAPIError{Code: "PreconditionFailed"}, precondition: true},
		{name: "precondition status", err: responseErr(http.StatusPreconditionFailed), precondition: true},
		{name: "access denied", err: &smithy.GenericAPIError{Code: "AccessDenied"}},
		{name: "wrapped precondition", err: fmt.Errorf("delete: %w", &smithy.GenericAPIError{Code: "PreconditionFailed"}), precondition: true},
		{name: "unrelated", err: errors.New("boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.notFound, isS3NotFound(tc.err))
			require.Equal(t, tc.precondition, isS3PreconditionFailed(tc.err))
		})
	}
}

// TestAzureCloudForEnvironment pins the mapping cleanup uses to locate state. Terraform resolves the
// azurerm backend endpoint from ARM_ENVIRONMENT, so a mismatch here would delete from the wrong cloud
// and silently orphan state on sovereign installations.
func TestAzureCloudForEnvironment(t *testing.T) {
	for _, tc := range []struct {
		environment string
		suffix      string
		audience    string
	}{
		{"", "blob.core.windows.net", cloud.AzurePublic.Services[cloud.ResourceManager].Audience},
		{"public", "blob.core.windows.net", cloud.AzurePublic.Services[cloud.ResourceManager].Audience},
		{"AzurePublicCloud", "blob.core.windows.net", cloud.AzurePublic.Services[cloud.ResourceManager].Audience},
		{" usgovernment ", "blob.core.usgovcloudapi.net", cloud.AzureGovernment.Services[cloud.ResourceManager].Audience},
		{"USGovernmentCloud", "blob.core.usgovcloudapi.net", cloud.AzureGovernment.Services[cloud.ResourceManager].Audience},
		{"china", "blob.core.chinacloudapi.cn", cloud.AzureChina.Services[cloud.ResourceManager].Audience},
	} {
		t.Run(tc.environment, func(t *testing.T) {
			config, suffix, err := azureCloudForEnvironment(tc.environment)
			require.NoError(t, err)
			require.Equal(t, tc.suffix, suffix)
			require.Equal(t, tc.audience, config.Services[cloud.ResourceManager].Audience)
		})
	}

	// An unknown value must not silently fall back to public Azure.
	for _, environment := range []string{"german", "notacloud"} {
		t.Run("unsupported/"+environment, func(t *testing.T) {
			_, _, err := azureCloudForEnvironment(environment)
			require.ErrorContains(t, err, "unsupported ARM_ENVIRONMENT")
			require.ErrorContains(t, err, environment)
		})
	}
}

// TestAzureBackendAuthCarriesEnvironment verifies ARM_ENVIRONMENT reaches cleanup for both Azure
// credential kinds, since it is read from the merged execution environment rather than os.Environ.
func TestAzureBackendAuthCarriesEnvironment(t *testing.T) {
	for _, federated := range []bool{false, true} {
		t.Run(fmt.Sprintf("federated=%v", federated), func(t *testing.T) {
			env := map[string]string{"ARM_ENVIRONMENT": "usgovernment"}
			auth, err := setAzureBackendAuth(backendTestAzureCredential(federated), env)
			require.NoError(t, err)
			require.Equal(t, "usgovernment", auth.AzureEnvironment)
			// The variable selects a cloud rather than a credential, so it must survive scrubbing.
			require.Equal(t, "usgovernment", env["ARM_ENVIRONMENT"])
		})
	}
}
