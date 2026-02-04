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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/metrics"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// ErrRecipeNameEmpty is the error when the recipe name is empty.
	ErrRecipeNameEmpty = errors.New("recipe name cannot be empty")
)

var _ TerraformExecutor = (*executor)(nil)

// NewExecutor creates a new Executor with the given UCP connection and secret provider, to execute a Terraform recipe.
func NewExecutor(ucpConn sdk.Connection, secretProvider *secretprovider.SecretProvider, kubernetesClients kubernetesclientprovider.KubernetesClientProvider) *executor {
	return &executor{ucpConn: ucpConn, secretProvider: secretProvider, kubernetesClients: kubernetesClients}
}

type executor struct {
	// ucpConn represents the configuration needed to connect to UCP, required to fetch cloud provider credentials.
	ucpConn sdk.Connection

	// secretProvider is the secret store provider used for managing credentials in UCP.
	secretProvider *secretprovider.SecretProvider

	// kubernetesClients provides access to the Kubernetes clients.
	kubernetesClients kubernetesclientprovider.KubernetesClientProvider
}

// Deploy ensures Terraform is available, creates a working directory, generates a config, and runs Terraform init and
// apply in the working directory, returning an error if any of these steps fail.
func (e *executor) Deploy(ctx context.Context, options Options) (*tfjson.State, error) {
	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, TerraformPath: options.TerraformPath, LogLevel: options.LogLevel})
	if err != nil {
		return nil, err
	}

	// Create Terraform config in the working directory
	configResult, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	if options.EnvConfig != nil {
		// Set environment variables for the Terraform process.
		err = e.setEnvironmentVariables(tf, options, configResult.terraformRCPath)
		if err != nil {
			return nil, err
		}
	}

	// Run TF Init and Apply in the working directory
	stateLockTimeout := getStateLockTimeout(options.StateLockTimeout)
	state, err := initAndApply(ctx, tf, stateLockTimeout)
	if err != nil {
		return nil, err
	}

	if configResult.usesKubernetesBackend {
		// Validate that the terraform state file backend source exists.
		// Currently only Kubernetes secret backend is supported, which is created by Terraform as a part of Terraform apply.
		kubernetesClient, err := e.kubernetesClients.ClientGoClient()
		if err != nil {
			return nil, fmt.Errorf("error getting kubernetes client: %w", err)
		}

		backendExists, err := backends.NewKubernetesBackend(kubernetesClient).ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+configResult.kubernetesBackendSuffix)
		if err != nil {
			return nil, fmt.Errorf("error retrieving kubernetes secret for terraform state: %w", err)
		} else if !backendExists {
			return nil, errors.New("expected kubernetes secret for terraform state is not found")
		}
	}

	return state, nil
}

// Delete ensures Terraform is available, creates a working directory, generates a config, and runs Terraform destroy
// in the working directory, returning an error if any of these steps fail.
func (e *executor) Delete(ctx context.Context, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, TerraformPath: options.TerraformPath, LogLevel: options.LogLevel})
	// Note: We use a global shared binary approach, so we should NOT call i.Remove()
	// as it would remove the shared global binary that other operations might be using.
	// The global binary will persist across operations to eliminate race conditions.
	if err != nil {
		return err
	}

	// Create Terraform config in the working directory
	configResult, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return err
	}

	if options.EnvConfig != nil {
		// Set environment variables for the Terraform process.
		if err := e.setEnvironmentVariables(tf, options, configResult.terraformRCPath); err != nil {
			return err
		}
	}

	if configResult.usesKubernetesBackend {
		// Before running terraform init and destroy, ensure that the Terraform state file storage source exists.
		// If the state file source has been deleted or wasn't created due to a failure during apply then
		// terraform initialization will fail due to missing backend source.
		kubernetesClient, err := e.kubernetesClients.ClientGoClient()
		if err != nil {
			return fmt.Errorf("error getting kubernetes client: %w", err)
		}

		backendExists, err := backends.NewKubernetesBackend(kubernetesClient).ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+configResult.kubernetesBackendSuffix)
		if err != nil {
			// Continue with the delete flow for all errors other than backend not found.
			// If it is an intermittent error then the delete flow will fail and should be retried from the client.
			logger.Info(fmt.Sprintf("Error retrieving Terraform state file backend: %s", err.Error()))
		} else if !backendExists {
			// Skip deletion if the backend does not exist. Delete can't be performed without Terraform state file.
			logger.Info("Skipping deletion of recipe resources: Terraform state file backend does not exist.")
			return nil
		}
	}

	// Run TF Destroy in the working directory to delete the resources deployed by the recipe
	stateLockTimeout := getStateLockTimeout(options.StateLockTimeout)
	err = initAndDestroy(ctx, tf, stateLockTimeout)
	if err != nil {
		return err
	}

	if configResult.usesKubernetesBackend {
		// Delete the kubernetes secret created for terraform state file.
		kubernetesClient, err := e.kubernetesClients.ClientGoClient()
		if err != nil {
			return fmt.Errorf("error getting kubernetes client: %w", err)
		}
		err = kubernetesClient.CoreV1().
			Secrets(backends.RadiusNamespace).
			Delete(ctx, backends.KubernetesBackendNamePrefix+configResult.kubernetesBackendSuffix, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("error deleting kubernetes secret for terraform state: %w", err)
		}
	}

	return nil
}

func (e *executor) GetRecipeMetadata(ctx context.Context, options Options) (map[string]any, error) {
	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, TerraformPath: options.TerraformPath, LogLevel: options.LogLevel})
	if err != nil {
		return nil, err
	}

	_, err = getTerraformConfig(ctx, tf.WorkingDir(), options)
	if err != nil {
		return nil, err
	}

	result, err := downloadAndInspect(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"parameters": result.Parameters,
	}, nil
}

// setEnvironmentVariables sets environment variables for the Terraform process by reading values from the recipe configuration.
// Terraform process will use environment variables as input for the recipe deployment.
func (e executor) setEnvironmentVariables(tf *tfexec.Terraform, options Options, terraformRCPath string) error {
	if options.EnvConfig == nil {
		return nil
	}

	// Populate envVars with the environment variables from current process
	envVars := splitEnvVar(os.Environ())
	recipeConfig := &options.EnvConfig.RecipeConfig
	var envVarUpdate bool

	if len(recipeConfig.Env.AdditionalProperties) > 0 {
		envVarUpdate = true
		for key, value := range recipeConfig.Env.AdditionalProperties {
			envVars[key] = value
		}
	}

	if len(recipeConfig.EnvSecrets) > 0 {
		for secretName, secretReference := range recipeConfig.EnvSecrets {
			// Extract secret value from the secrets input
			if secretData, ok := options.Secrets[secretReference.Source]; ok {
				if secretValue, ok := secretData.Data[secretReference.Key]; ok {
					envVarUpdate = true
					envVars[secretName] = secretValue
				} else {
					return fmt.Errorf("missing secret key in secret store id: %s", secretReference.Source)
				}
			} else {
				return fmt.Errorf("missing secret source: %s", secretReference.Source)
			}
		}
	}

	// Apply TerraformSettings.Env if configured
	if options.EnvConfig.TerraformSettings != nil && len(options.EnvConfig.TerraformSettings.Env) > 0 {
		envVarUpdate = true
		for key, value := range options.EnvConfig.TerraformSettings.Env {
			envVars[key] = value
		}
	}

	// Apply TerraformSettings.Logging (TF_LOG, TF_LOG_PATH) if configured
	if options.EnvConfig.TerraformSettings != nil && options.EnvConfig.TerraformSettings.Logging != nil {
		logging := options.EnvConfig.TerraformSettings.Logging
		if logging.Level != "" {
			envVarUpdate = true
			envVars["TF_LOG"] = string(logging.Level)
		}
		if logging.Path != "" {
			envVarUpdate = true
			envVars["TF_LOG_PATH"] = logging.Path
		}
	}

	// Apply TerraformSettings.TerraformRC (TF_CLI_CONFIG_FILE) if configured
	if terraformRCPath != "" {
		envVarUpdate = true
		envVars["TF_CLI_CONFIG_FILE"] = terraformRCPath
	}

	// Set the environment variables for the Terraform process
	if envVarUpdate {
		if err := tf.SetEnv(envVars); err != nil {
			return fmt.Errorf("failed to set environment variables: %w", err)
		}
	}

	return nil
}

// splitEnvVar splits a slice of environment variables into a map of keys and values.
func splitEnvVar(envVars []string) map[string]string {
	parsedEnvVars := make(map[string]string)
	for _, item := range envVars {
		splits := strings.SplitN(item, "=", 2) // Split on the first "="
		if len(splits) == 2 {
			parsedEnvVars[splits[0]] = splits[1]
		}
	}

	return parsedEnvVars
}

// generateConfig generates Terraform configuration with required inputs for the module, providers and backend to be initialized and applied.
type generateConfigResult struct {
	kubernetesBackendSuffix string
	usesKubernetesBackend   bool
	terraformRCPath         string
}

func (e *executor) generateConfig(ctx context.Context, tf *tfexec.Terraform, options Options) (generateConfigResult, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	workingDir := tf.WorkingDir()

	tfConfig, err := getTerraformConfig(ctx, workingDir, options)
	if err != nil {
		return generateConfigResult{}, err
	}

	loadedModule, err := downloadAndInspect(ctx, tf, options)
	if err != nil {
		return generateConfigResult{}, err
	}

	// Generate Terraform providers configuration for required providers and add it to the Terraform configuration.
	logger.Info(fmt.Sprintf("Adding provider config for required providers %+v", loadedModule.RequiredProviders))
	if err := tfConfig.AddProviders(ctx, loadedModule.RequiredProviders, providers.GetUCPConfiguredTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig, options.Secrets); err != nil {
		return generateConfigResult{}, err
	}

	// Configure backend: use custom backend from TerraformSettings if specified, otherwise use Kubernetes backend
	var backendConfig map[string]any
	result := generateConfigResult{}

	if options.EnvConfig != nil && options.EnvConfig.TerraformSettings != nil && options.EnvConfig.TerraformSettings.Backend != nil {
		// Use custom backend from TerraformSettings
		customBackend := options.EnvConfig.TerraformSettings.Backend
		logger.Info(fmt.Sprintf("Using custom backend type: %s", customBackend.Type))
		_, err = tfConfig.AddCustomBackend(customBackend.Type, customBackend.Config)
		if err != nil {
			return generateConfigResult{}, fmt.Errorf("error adding custom backend: %w", err)
		}
	} else {
		// Use default Kubernetes backend
		kubernetesClient, err := e.kubernetesClients.ClientGoClient()
		if err != nil {
			return generateConfigResult{}, fmt.Errorf("error getting kubernetes client: %w", err)
		}

		backendConfig, err = tfConfig.AddTerraformBackend(options.ResourceRecipe, backends.NewKubernetesBackend(kubernetesClient))
		if err != nil {
			return generateConfigResult{}, err
		}
		result.usesKubernetesBackend = true

		// Retrieving the secret_suffix property from backend config to use it to verify secret creation during terraform init.
		// This is only used for the backend of type kubernetes.
		if backendDetails, ok := backendConfig[backends.BackendKubernetes]; ok {
			backendMap := backendDetails.(map[string]any)
			if secret, ok := backendMap["secret_suffix"]; ok {
				result.kubernetesBackendSuffix = secret.(string)
			}
		}
	}

	// Write .terraformrc if TerraformSettings.TerraformRC is configured
	if options.EnvConfig != nil && options.EnvConfig.TerraformSettings != nil && options.EnvConfig.TerraformSettings.TerraformRC != nil {
		terraformrcPath, err := e.writeTerraformRC(ctx, workingDir, options.EnvConfig.TerraformSettings.TerraformRC, options.Secrets)
		if err != nil {
			return generateConfigResult{}, fmt.Errorf("error writing .terraformrc: %w", err)
		}
		logger.Info(fmt.Sprintf("Written .terraformrc to: %s", terraformrcPath))
		result.terraformRCPath = terraformrcPath
	}

	// Add recipe context parameter to the generated Terraform config's module parameters.
	// This should only be added if the recipe context variable is declared in the downloaded module.
	if loadedModule.ContextVarExists {
		logger.Info("Adding recipe context module result")

		// Create the recipe context object to be passed to the recipe deployment
		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return generateConfigResult{}, err
		}

		//update the recipe context with connected resources properties
		if options.ResourceRecipe != nil {
			recipectx.Resource.Connections = options.ResourceRecipe.ConnectedResourcesProperties
		}

		if err = tfConfig.AddRecipeContext(ctx, options.EnvRecipe.Name, recipectx); err != nil {
			return generateConfigResult{}, err
		}
	}
	if loadedModule.ResultOutputExists {
		if err = tfConfig.AddOutputs(options.EnvRecipe.Name); err != nil {
			return generateConfigResult{}, err
		}
	}

	// Add more configurations here.

	// Ensure that we need to save the configuration after adding providers and recipecontext.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return generateConfigResult{}, err
	}

	return result, nil
}

// getTerraformConfig initializes the Terraform json config with provided module source and saves it
func getTerraformConfig(ctx context.Context, workingDir string, options Options) (*config.TerraformConfig, error) {
	// Generate Terraform json config in the working directory
	// Use recipe name as a local reference to the module.
	// Modules are downloaded in a subdirectory in the working directory. Name of the module specified in the
	// configuration is used as subdirectory name under .terraform/modules directory.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	localModuleName := options.EnvRecipe.Name
	if localModuleName == "" {
		return nil, ErrRecipeNameEmpty
	}

	// Create Terraform configuration containing module information with the given recipe parameters.
	tfConfig, err := config.New(ctx, localModuleName, options.EnvRecipe, options.ResourceRecipe)
	if err != nil {
		return nil, err
	}

	// Before downloading the module, Teraform configuration needs to be persisted in the working directory.
	// Terraform Get command uses this config file to download module from the source specified in the config.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return nil, err
	}

	return tfConfig, nil
}

// getStateLockTimeout returns the configured state lock timeout or the default if not set.
func getStateLockTimeout(timeout string) string {
	if timeout == "" {
		return DefaultStateLockTimeout
	}
	return timeout
}

// writeTerraformRC writes a .terraformrc file in the working directory with provider installation and credentials configuration.
// Returns the path to the written file.
func (e *executor) writeTerraformRC(ctx context.Context, workingDir string, tfrc *datamodel.TerraformCliConfiguration, secrets map[string]recipes.SecretData) (string, error) {
	// Build the terraformrc content
	var content strings.Builder

	// Add provider_installation block if configured
	if tfrc.ProviderInstallation != nil {
		content.WriteString("provider_installation {\n")

		// Add network_mirror block
		if tfrc.ProviderInstallation.NetworkMirror != nil {
			nm := tfrc.ProviderInstallation.NetworkMirror
			content.WriteString("  network_mirror {\n")
			content.WriteString(fmt.Sprintf("    url = \"%s\"\n", nm.URL))

			if len(nm.Include) > 0 {
				content.WriteString("    include = [")
				for i, inc := range nm.Include {
					if i > 0 {
						content.WriteString(", ")
					}
					content.WriteString(fmt.Sprintf("\"%s\"", inc))
				}
				content.WriteString("]\n")
			}

			if len(nm.Exclude) > 0 {
				content.WriteString("    exclude = [")
				for i, exc := range nm.Exclude {
					if i > 0 {
						content.WriteString(", ")
					}
					content.WriteString(fmt.Sprintf("\"%s\"", exc))
				}
				content.WriteString("]\n")
			}

			content.WriteString("  }\n")
		}

		// Add direct block
		if tfrc.ProviderInstallation.Direct != nil {
			d := tfrc.ProviderInstallation.Direct
			content.WriteString("  direct {\n")

			if len(d.Include) > 0 {
				content.WriteString("    include = [")
				for i, inc := range d.Include {
					if i > 0 {
						content.WriteString(", ")
					}
					content.WriteString(fmt.Sprintf("\"%s\"", inc))
				}
				content.WriteString("]\n")
			}

			if len(d.Exclude) > 0 {
				content.WriteString("    exclude = [")
				for i, exc := range d.Exclude {
					if i > 0 {
						content.WriteString(", ")
					}
					content.WriteString(fmt.Sprintf("\"%s\"", exc))
				}
				content.WriteString("]\n")
			}

			content.WriteString("  }\n")
		}

		content.WriteString("}\n\n")
	}

	// Add credentials blocks if configured
	if len(tfrc.Credentials) > 0 {
		for hostname, cred := range tfrc.Credentials {
			if cred.Token != nil {
				// Resolve the token from secrets.
				// If resolution fails, we return an error because the user explicitly configured
				// these credentials - they are required for registry/host authentication.
				tokenValue, err := resolveSecretRef(cred.Token, secrets)
				if err != nil {
					return "", fmt.Errorf("failed to resolve credential token for hostname %q: %w", hostname, err)
				}

				// Escape hostname and token values to prevent HCL injection and ensure valid syntax.
				// This handles special characters like quotes, backslashes, and newlines.
				content.WriteString(fmt.Sprintf("credentials \"%s\" {\n", escapeHCLString(hostname)))
				content.WriteString(fmt.Sprintf("  token = \"%s\"\n", escapeHCLString(tokenValue)))
				content.WriteString("}\n\n")
			}
		}
	}

	// Write the file
	terraformrcPath := fmt.Sprintf("%s/.terraformrc", workingDir)
	if err := os.WriteFile(terraformrcPath, []byte(content.String()), 0600); err != nil {
		return "", fmt.Errorf("error writing .terraformrc: %w", err)
	}

	return terraformrcPath, nil
}

// resolveSecretRef resolves a secret reference to its actual value from the secrets map.
func resolveSecretRef(ref *datamodel.SecretRef, secrets map[string]recipes.SecretData) (string, error) {
	if ref == nil {
		return "", errors.New("secret reference is nil")
	}

	secretData, ok := secrets[ref.SecretID]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", ref.SecretID)
	}

	value, ok := secretData.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key '%s' not found in secret '%s'", ref.Key, ref.SecretID)
	}

	return value, nil
}

// escapeHCLString escapes special characters in a string for safe inclusion in an HCL quoted string.
// This prevents injection attacks and ensures the .terraformrc file is valid HCL.
// Escaped characters: backslash, double quote, newline, carriage return, tab.
func escapeHCLString(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			result.WriteRune(r)
		}
	}

	return result.String()
}

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, tf *tfexec.Terraform, stateLockTimeout string) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return nil, fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
		[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState)})

	// Apply Terraform configuration with state lock timeout
	logger.Info("Running Terraform apply with state lock timeout: " + stateLockTimeout)
	if err := tf.Apply(ctx, tfexec.Lock(true), tfexec.LockTimeout(stateLockTimeout)); err != nil {
		return nil, fmt.Errorf("terraform apply failure: %w", err)
	}

	// Load Terraform state to retrieve the outputs
	logger.Info("Fetching Terraform state")

	// Verify terraform binary is still accessible before state operation
	if execPath := tf.ExecPath(); execPath != "" {
		if _, err := os.Stat(execPath); err != nil {
			logger.Info(fmt.Sprintf("ERROR: Terraform binary missing at state fetch: %s", err.Error()))
			return nil, fmt.Errorf("terraform binary file not found: %w", err)
		}
	}

	return tf.Show(ctx)
}

// initAndDestroy runs Terraform init and destroy in the provided working directory.
func initAndDestroy(ctx context.Context, tf *tfexec.Terraform, stateLockTimeout string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime, nil)

	// Destroy Terraform configuration with state lock timeout
	logger.Info("Running Terraform destroy with state lock timeout: " + stateLockTimeout)
	if err := tf.Destroy(ctx, tfexec.Lock(true), tfexec.LockTimeout(stateLockTimeout)); err != nil {
		return fmt.Errorf("terraform destroy failure: %w", err)
	}

	return nil
}
