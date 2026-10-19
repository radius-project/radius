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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"time"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	goretry "github.com/sethvargo/go-retry"

	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/metrics"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/paramresolver"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/retry"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// ErrRecipeNameEmpty is the error when the recipe name is empty.
	ErrRecipeNameEmpty = errors.New("recipe name cannot be empty")
)

// outputMappingValidationMode controls declaration validation during configuration generation.
type outputMappingValidationMode int

const (
	requireValidOutputMappings outputMappingValidationMode = iota
	skipOutputMappingValidation
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

	awsCredentials   credentials.CredentialProvider[credentials.AWSCredential]
	azureCredentials credentials.CredentialProvider[credentials.AzureCredential]

	// deleteStateObject removes a cloud backend's state object after destroy. It is overridden in
	// tests so that they do not reach a real bucket or container.
	deleteStateObject func(ctx context.Context, settings *datamodel.TerraformBackend, auth backends.CloudBackendAuth, key string) error

	// stateEndpointOverride redirects cloud state cleanup at a test storage endpoint. It is empty in
	// production, where the endpoint comes from the backend's region or storage account, and is set
	// only by tests that exercise the real storage SDKs against a stub server.
	stateEndpointOverride string

	// stateTransport carries the HTTP transport those same tests use to keep the Azure blob and token
	// requests inside the stub server. It is nil in production.
	stateTransport policy.Transporter
}

// backendResult carries the extra lifecycle operations each backend needs after Terraform runs.
type backendResult struct {
	config *config.TerraformConfig

	// kubernetesSecretName is set only for the Kubernetes backend, whose state secret Radius manages.
	kubernetesSecretName string

	// cloudBackendType and cloudStateKey identify the cloud state object Terraform wrote.
	cloudBackendType string
	cloudStateKey    string
}

// Deploy ensures Terraform is available, creates a working directory, generates a config, and runs Terraform init and
// apply in the working directory, returning an error if any of these steps fail.
func (e *executor) Deploy(ctx context.Context, options Options) (*tfjson.State, error) {
	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, LogLevel: options.LogLevel})
	if err != nil {
		return nil, err
	}

	// Set environment variables (including TF_CLI_CONFIG_FILE for the
	// generated .terraformrc) before generateConfig, because generateConfig
	// runs `terraform get` to download the module. Module downloads from
	// authenticated registries need credentials and provider_installation
	// rules in effect at fetch time, not just at apply time.
	auth, err := e.prepareExecution(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	// Create Terraform config in the working directory
	backend, err := e.generateConfig(ctx, tf, options, auth, requireValidOutputMappings)
	if err != nil {
		return nil, err
	}

	// Run TF Init and Apply in the working directory
	stateLockTimeout := getStateLockTimeout(options.StateLockTimeout)
	state, err := initAndApply(ctx, tf, stateLockTimeout)
	if err != nil {
		return nil, err
	}

	if backend.kubernetesSecretName == "" {
		return state, nil
	}

	// Kubernetes creates the state secret during apply. Cloud errors are handled by Terraform.
	kubernetesClient, err := e.kubernetesClients.ClientGoClient()
	if err != nil {
		return nil, fmt.Errorf("error getting kubernetes client: %w", err)
	}

	backendExists, err := backends.NewKubernetesBackend(kubernetesClient).ValidateBackendExists(ctx, backend.kubernetesSecretName)
	if err != nil {
		return nil, fmt.Errorf("error retrieving kubernetes secret for terraform state: %w", err)
	} else if !backendExists {
		return nil, errors.New("expected kubernetes secret for terraform state is not found")
	}

	return state, nil
}

// Delete ensures Terraform is available, creates a working directory, generates a config, and runs Terraform destroy
// in the working directory, returning an error if any of these steps fail.
func (e *executor) Delete(ctx context.Context, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, LogLevel: options.LogLevel})
	// Note: We use a global shared binary approach, so we should NOT call i.Remove()
	// as it would remove the shared global binary that other operations might be using.
	// The global binary will persist across operations to eliminate race conditions.
	if err != nil {
		return err
	}

	auth, err := e.prepareExecution(ctx, tf, options)
	if err != nil {
		return err
	}

	// Create Terraform config after setting registry and backend authentication.
	backend, err := e.generateConfig(ctx, tf, options, auth, skipOutputMappingValidation)
	if err != nil {
		return err
	}

	stateLockTimeout := getStateLockTimeout(options.StateLockTimeout)
	if backend.kubernetesSecretName == "" {
		// Native cloud backends handle missing state, locking and permissions.
		if err := initAndDestroy(ctx, tf, stateLockTimeout, backend.config, options.EnvRecipe.Name); err != nil {
			return err
		}

		// Terraform leaves an empty state object behind after destroy. Remove it so deleting a
		// Radius resource does not accumulate orphaned objects in the user's bucket or container.
		// This is best effort: the resources are already destroyed, so a cleanup failure must not
		// fail the delete. Operators can remove leftovers using the logged state key.
		e.deleteCloudState(ctx, options.EnvConfig.TerraformBackend, auth, backend.cloudStateKey)
		return nil
	}

	// Before running terraform init and destroy, ensure that the Terraform state file storage source exists.
	// If the state file source has been deleted or wasn't created due to a failure during apply then
	// terraform initialization will fail due to missing backend source.
	kubernetesClient, err := e.kubernetesClients.ClientGoClient()
	if err != nil {
		return fmt.Errorf("error getting kubernetes client: %w", err)
	}

	backendExists, err := backends.NewKubernetesBackend(kubernetesClient).ValidateBackendExists(ctx, backend.kubernetesSecretName)
	if err != nil {
		// Continue with the delete flow for all errors other than backend not found.
		// If it is an intermittent error then the delete flow will fail and should be retried from the client.
		logger.Info(fmt.Sprintf("Error retrieving Terraform state file backend: %s", err.Error()))
	} else if !backendExists {
		// Skip deletion if the backend does not exist. Delete can't be performed without Terraform state file.
		logger.Info("Skipping deletion of recipe resources: Terraform state file backend does not exist.")
		return nil
	}

	// Run TF Destroy in the working directory to delete the resources deployed by the recipe
	err = initAndDestroy(ctx, tf, stateLockTimeout, backend.config, options.EnvRecipe.Name)
	if err != nil {
		return err
	}

	// Delete the kubernetes secret created for terraform state file.
	err = kubernetesClient.CoreV1().
		Secrets(backends.RadiusNamespace).
		Delete(ctx, backend.kubernetesSecretName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting kubernetes secret for terraform state: %w", err)
	}

	return nil
}

func (e *executor) GetRecipeMetadata(ctx context.Context, options Options) (map[string]any, error) {
	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, InstallOptions{RootDir: options.RootDir, LogLevel: options.LogLevel})
	if err != nil {
		return nil, err
	}

	tfConfig, err := getTerraformConfig(ctx, tf.WorkingDir(), options)
	if err != nil {
		return nil, err
	}

	result, err := downloadAndInspect(ctx, tf, tfConfig, options)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"parameters": result.Parameters,
	}, nil
}

// prepareExecution merges all process configuration once, before module fetching and init. It returns
// any non-secret backend authentication to be rendered into the generated backend configuration.
func (e executor) prepareExecution(ctx context.Context, tf *tfexec.Terraform, options Options) (backends.CloudBackendAuth, error) {
	var auth backends.CloudBackendAuth
	if options.EnvConfig == nil {
		return auth, nil
	}

	// Populate envVars with the environment variables from current process
	envVars := splitEnvVar(os.Environ())
	recipeConfig := &options.EnvConfig.RecipeConfig
	var envVarUpdate bool

	if len(recipeConfig.Env.AdditionalProperties) > 0 {
		envVarUpdate = true
		maps.Copy(envVars, recipeConfig.Env.AdditionalProperties)
	}

	if len(recipeConfig.EnvSecrets) > 0 {
		for secretName, secretReference := range recipeConfig.EnvSecrets {
			// Extract secret value from the secrets input
			if secretData, ok := options.Secrets[secretReference.Source]; ok {
				if secretValue, ok := secretData.Data[secretReference.Key]; ok {
					envVarUpdate = true
					envVars[secretName] = secretValue
				} else {
					return auth, fmt.Errorf("missing secret key in secret store id: %s", secretReference.Source)
				}
			} else {
				return auth, fmt.Errorf("missing secret source: %s", secretReference.Source)
			}
		}
	}

	// Render a .terraformrc for provider_installation rules and credentials, and
	// point Terraform at it. Both inputs are populated only by the Radius.Core
	// path; Applications.Core leaves them nil/empty so this is a no-op there.
	if recipeConfig.Terraform.ProviderInstallation != nil || len(recipeConfig.Terraform.Credentials) > 0 {
		rcPath, err := writeTerraformCLIConfig(
			tf.WorkingDir(),
			recipeConfig.Terraform.ProviderInstallation,
			recipeConfig.Terraform.Credentials,
			options.Secrets,
		)
		if err != nil {
			return auth, err
		}
		if rcPath != "" {
			envVarUpdate = true
			envVars[envTFCLIConfigFile] = rcPath
		}
	}

	// Set the environment variables for the Terraform process
	if options.EnvConfig.TerraformBackend != nil {
		var err error
		auth, err = e.resolveBackendAuth(ctx, options.EnvConfig.TerraformBackend, tf.WorkingDir(), envVars)
		if err != nil {
			return auth, err
		}
		envVarUpdate = true
	}
	if envVarUpdate {
		if err := tf.SetEnv(envVars); err != nil {
			return auth, fmt.Errorf("failed to set environment variables: %w", err)
		}
	}

	return auth, nil
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
func (e *executor) generateConfig(ctx context.Context, tf *tfexec.Terraform, options Options, auth backends.CloudBackendAuth, validationMode outputMappingValidationMode) (*backendResult, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	workingDir := tf.WorkingDir()

	tfConfig, err := getTerraformConfig(ctx, workingDir, options)
	if err != nil {
		return nil, err
	}

	loadedModule, err := downloadAndInspect(ctx, tf, tfConfig, options)
	if err != nil {
		return nil, err
	}

	useOutputMappings := usesOutputMappings(options.EnvRecipe, loadedModule, validationMode)
	if useOutputMappings {
		if err := validateOutputMappings(options.EnvRecipe, loadedModule, validationMode); err != nil {
			return nil, err
		}
	}

	// Generate Terraform providers configuration for required providers and add it to the Terraform configuration.
	logger.Info(fmt.Sprintf("Adding provider config for required providers %+v", loadedModule.RequiredProviders))
	if err := tfConfig.AddProviders(ctx, loadedModule.RequiredProviders, providers.GetUCPConfiguredTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig, options.Secrets); err != nil {
		return nil, err
	}

	var backendBuilder backends.Builder
	if options.EnvConfig != nil && options.EnvConfig.TerraformBackend != nil {
		backendBuilder = backends.CloudBackend{Settings: options.EnvConfig.TerraformBackend, Auth: auth}
	} else {
		kubernetesClient, err := e.kubernetesClients.ClientGoClient()
		if err != nil {
			return nil, fmt.Errorf("error getting kubernetes client: %w", err)
		}
		backendBuilder = backends.NewKubernetesBackend(kubernetesClient)
	}

	backendConfig, err := tfConfig.AddTerraformBackend(options.ResourceRecipe, backendBuilder)
	if err != nil {
		return nil, err
	}

	result := &backendResult{config: tfConfig}
	if backendDetails, ok := backendConfig[backends.BackendKubernetes]; ok {
		backendMap := backendDetails.(map[string]any)
		if secret, ok := backendMap["secret_suffix"]; ok {
			result.kubernetesSecretName = backends.KubernetesBackendNamePrefix + secret.(string)
		}
	}
	// The state key is an opaque hash, so record it: it is the only way an operator can map an
	// object in their bucket or container back to a Radius resource. It holds no credential.
	if backendType, key := backends.StateKey(backendConfig); key != "" {
		result.cloudBackendType = backendType
		result.cloudStateKey = key
		logger.Info(fmt.Sprintf("Using Terraform %s backend with state key %q", backendType, key))
	}

	// Add recipe context parameter to the generated Terraform config's module parameters.
	// This should only be added if the recipe context variable is declared in the downloaded module.
	if loadedModule.ContextVarExists {
		logger.Info("Adding recipe context module result")

		// Create the recipe context object to be passed to the recipe deployment
		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return nil, err
		}

		//update the recipe context with connected resources properties
		if options.ResourceRecipe != nil {
			recipectx.Resource.Connections = options.ResourceRecipe.ConnectedResourcesProperties
		}

		if err = tfConfig.AddRecipeContext(ctx, options.EnvRecipe.Name, recipectx); err != nil {
			return nil, err
		}
	} else {
		// Direct module path: the module does not declare a recipe context variable, so instead of
		// injecting context we resolve {{context.*}} expressions embedded in the recipe parameters.
		logger.Info("Direct module detected, resolving parameter expressions")

		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return nil, err
		}

		if options.ResourceRecipe != nil {
			recipectx.Resource.Connections = options.ResourceRecipe.ConnectedResourcesProperties
		}

		// Merge environment-level and resource-level parameters (resource parameters take precedence),
		// then resolve any {{context.*}} expressions against the recipe context.
		var resourceParams map[string]any
		if options.ResourceRecipe != nil {
			resourceParams = options.ResourceRecipe.Parameters
		}
		mergedParams := recipes_util.ShallowMergeParameters(options.EnvRecipe.Parameters, resourceParams)
		resolvedParams := paramresolver.ResolveParameterExpressions(mergedParams, recipectx)
		if resolvedParams != nil {
			tfConfig.Module[options.EnvRecipe.Name].SetParams(config.RecipeParams(resolvedParams))
		}
	}
	if useOutputMappings {
		// Explicit mappings take precedence over a wrapped result output during deployment, matching
		// prepareRecipeResponse. Generate each referenced output so it is available in Terraform state.
		if err = tfConfig.AddMappedOutputs(options.EnvRecipe.Name, options.EnvRecipe.Outputs, loadedModule.OutputSensitivity, false); err != nil {
			return nil, err
		}
		// SecretOutputs are always treated as secrets — force their generated output blocks sensitive so
		// a module output the module did not itself mark sensitive (e.g. AVM primaryConnectionString) is
		// still redacted in Terraform's stdout/stderr, which Radius streams into logs.
		if err = tfConfig.AddMappedOutputs(options.EnvRecipe.Name, options.EnvRecipe.SecretOutputs, loadedModule.OutputSensitivity, true); err != nil {
			return nil, err
		}
	} else if loadedModule.ResultOutputExists {
		if err = tfConfig.AddOutputs(options.EnvRecipe.Name); err != nil {
			return nil, err
		}
	} else {
		// Direct module without a mapping: re-export every module output so they are present in
		// the Terraform state and pass through unchanged (mirrors prepareRecipeResponse). Terraform
		// does not expose child module outputs as root outputs unless they are re-declared here.
		if err = tfConfig.AddAllOutputs(options.EnvRecipe.Name, loadedModule.OutputSensitivity); err != nil {
			return nil, err
		}
	}

	// Add more configurations here.

	if err := tfConfig.AddRecipeInputs(options.EnvRecipe.Name); err != nil {
		return nil, err
	}

	// Ensure that we need to save the configuration after adding providers and recipecontext.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return nil, err
	}

	return result, nil
}

func usesOutputMappings(definition *recipes.EnvironmentDefinition, module *moduleInspectResult, validationMode outputMappingValidationMode) bool {
	hasMappings := len(definition.Outputs) > 0 || len(definition.SecretOutputs) > 0
	return hasMappings && (validationMode == requireValidOutputMappings || !module.ResultOutputExists)
}

func validateOutputMappings(definition *recipes.EnvironmentDefinition, module *moduleInspectResult, validationMode outputMappingValidationMode) error {
	if validationMode == skipOutputMappingValidation {
		return nil
	}

	declaredOutputs := make([]string, 0, len(module.OutputSensitivity))
	for outputName := range module.OutputSensitivity {
		declaredOutputs = append(declaredOutputs, outputName)
	}

	return recipes_util.ValidateOutputsMapping(
		definition.Name,
		definition.ResourceType,
		declaredOutputs,
		definition.Outputs,
		definition.SecretOutputs)
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

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, tf *tfexec.Terraform, stateLockTimeout string) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := initializeTerraform(ctx, tf); err != nil {
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

	// Suppress stdout during tf.Show to prevent Terraform state (which may
	// contain sensitive values) from being written to the Radius logs.
	tf.SetStdout(io.Discard)
	defer tf.SetStdout(&tfLogWrapper{logger: logger})

	state, err := tf.Show(ctx)
	if err != nil {
		return nil, err
	}
	// Internal inputs must not reach recipe outputs or managed secret materialization.
	if state != nil && state.Values != nil {
		delete(state.Values.Outputs, config.RecipeInputsOutputName)
	}
	return state, nil
}

// initAndDestroy runs Terraform init and destroy in the provided working directory.
func initAndDestroy(ctx context.Context, tf *tfexec.Terraform, stateLockTimeout string, cfg *config.TerraformConfig, moduleName string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := initializeTerraform(ctx, tf); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime, nil)

	restored, err := restoreRecipeInputs(ctx, tf, cfg, moduleName)
	if err != nil {
		return err
	}

	// Destroy Terraform configuration with state lock timeout
	logger.Info("Running Terraform destroy with state lock timeout: " + stateLockTimeout)
	if err := tf.Destroy(ctx, tfexec.Lock(true), tfexec.LockTimeout(stateLockTimeout)); err != nil {
		if restored {
			preserveRecipeInputs(ctx, tf, stateLockTimeout)
		}
		return fmt.Errorf("terraform destroy failure: %w", err)
	}

	return nil
}

// preserveRecipeInputs writes the saved inputs back to state after a failed destroy.
// Terraform removes root outputs during a destroy apply, even when it fails. Without this,
// a retry has no saved inputs and fails validation on the redacted values.
// This is best effort: the destroy error is the actionable failure.
func preserveRecipeInputs(ctx context.Context, tf *tfexec.Terraform, stateLockTimeout string) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Restoring saved Terraform recipe inputs in state after destroy failure")
	err := tf.Apply(ctx, tfexec.RefreshOnly(true), tfexec.Lock(true), tfexec.LockTimeout(stateLockTimeout))
	if err != nil {
		logger.Error(err, "Failed to restore saved Terraform recipe inputs in state; a retry of the deletion may fail validation")
	}
}

// restoreRecipeInputs fills redacted module inputs from the saved state output. It reports
// whether saved inputs were found.
func restoreRecipeInputs(ctx context.Context, tf *tfexec.Terraform, cfg *config.TerraformConfig, moduleName string) (bool, error) {
	// Output -json exposes sensitive values. Keep them out of the Radius logs.
	tf.SetStdout(io.Discard)
	outputs, err := tf.Output(ctx)
	tf.SetStdout(&tfLogWrapper{logger: ucplog.FromContextOrDiscard(ctx)})
	if err != nil {
		return false, fmt.Errorf("reading terraform recipe inputs for deletion: %w", err)
	}
	output, ok := outputs[config.RecipeInputsOutputName]
	if !ok {
		// Older state has no saved inputs. Preserve its existing deletion behavior.
		return false, nil
	}
	var encoded string
	if err := json.Unmarshal(output.Value, &encoded); err != nil {
		return false, errors.New("invalid saved terraform recipe inputs: expected a string")
	}
	if err := cfg.RestoreRecipeInputs(moduleName, encoded); err != nil {
		return false, err
	}
	if err := cfg.Save(ctx, tf.WorkingDir()); err != nil {
		return false, err
	}
	return true, nil
}

func initializeTerraform(ctx context.Context, tf *tfexec.Terraform) error {
	backoff := goretry.WithMaxRetries(2, goretry.NewExponential(time.Second))
	retryer := retry.NewRetryer(&retry.RetryConfig{BackoffStrategy: backoff})
	return retryTerraformInit(ctx, retryer, func(ctx context.Context) error {
		return tf.Init(ctx)
	})
}

func retryTerraformInit(ctx context.Context, retryer *retry.Retryer, initFn func(context.Context) error) error {
	return retryer.RetryFunc(ctx, func(ctx context.Context) error {
		err := initFn(ctx)
		if err != nil && isTransientTerraformInitError(err) {
			return retry.RetryableError(err)
		}

		return err
	})
}

func isTransientTerraformInitError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}

	message := strings.ToLower(strings.Join(strings.Fields(err.Error()), " "))
	if !strings.Contains(message, "failed to request discovery document") {
		return false
	}

	transientErrors := []string{
		"client.timeout exceeded",
		"context deadline exceeded",
		"connection refused",
		"connection reset",
		"no such host",
		"temporary failure in name resolution",
		"tls handshake timeout",
		"unexpected eof",
	}
	for _, transientError := range transientErrors {
		if strings.Contains(message, transientError) {
			return true
		}
	}

	return false
}
