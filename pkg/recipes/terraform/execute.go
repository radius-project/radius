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
	"maps"
	"os"
	"path/filepath"
	"regexp"
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

// getEnvTerraformConfig extracts the terraform configuration from options, defaulting to empty if not configured
func getEnvTerraformConfig(options Options) datamodel.TerraformConfigProperties {
	if options.EnvConfig != nil {
		return options.EnvConfig.RecipeConfig.Terraform
	}
	return datamodel.TerraformConfigProperties{}
}

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

// Deploy installs Terraform, creates a working directory, generates a config, and runs Terraform init and
// apply in the working directory, returning an error if any of these steps fail.
func (e *executor) Deploy(ctx context.Context, options Options) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()

	terraformConfig := getEnvTerraformConfig(options)

	tf, err := Install(ctx, i, options.RootDir, terraformConfig, options.Secrets, options.LogLevel)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info("Failed to cleanup Terraform installation", "error", err.Error())
		}
	}()
	if err != nil {
		return nil, err
	}

	// Set environment variables for the Terraform process.
	err = e.setEnvironmentVariables(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	// Create Terraform config in the working directory
	kubernetesBackendSuffix, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	// Run TF Init and Apply in the working directory
	state, err := initAndApply(ctx, tf)
	if err != nil {
		return nil, err
	}

	// Validate that the terraform state file backend source exists.
	// Currently only Kubernetes secret backend is supported, which is created by Terraform as a part of Terraform apply.
	kubernetesClient, err := e.kubernetesClients.ClientGoClient()
	if err != nil {
		return nil, fmt.Errorf("error getting kubernetes client: %w", err)
	}

	backendExists, err := backends.NewKubernetesBackend(kubernetesClient).
		ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix)
	if err != nil {
		return nil, fmt.Errorf("error retrieving kubernetes secret for terraform state: %w", err)
	} else if !backendExists {
		return nil, errors.New("expected kubernetes secret for terraform state is not found")
	}

	return state, nil
}

// Delete installs Terraform, creates a working directory, generates a config, and runs Terraform destroy
// in the working directory, returning an error if any of these steps fail.
func (e *executor) Delete(ctx context.Context, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()

	terraformConfig := getEnvTerraformConfig(options)

	tf, err := Install(ctx, i, options.RootDir, terraformConfig, options.Secrets, options.LogLevel)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info("Failed to cleanup Terraform installation", "error", err.Error())
		}
	}()
	if err != nil {
		return err
	}

	// Set environment variables BEFORE generateConfig to ensure Git has proper TLS settings
	err = e.setEnvironmentVariables(ctx, tf, options)
	if err != nil {
		return err
	}

	// Create Terraform config in the working directory
	kubernetesBackendSuffix, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return err
	}

	// Before running terraform init and destroy, ensure that the Terraform state file storage source exists.
	// If the state file source has been deleted or wasn't created due to a failure during apply then
	// terraform initialization will fail due to missing backend source.
	kubernetesClient, err := e.kubernetesClients.ClientGoClient()
	if err != nil {
		return fmt.Errorf("error getting kubernetes client: %w", err)
	}

	backendExists, err := backends.NewKubernetesBackend(kubernetesClient).
		ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix)
	if err != nil {
		// Continue with the delete flow for all errors other than backend not found.
		// If it is an intermittent error then the delete flow will fail and should be retried from the client.
		logger.Info("Error retrieving Terraform state file backend", "error", err.Error())
	} else if !backendExists {
		// Skip deletion if the backend does not exist. Delete can't be performed without Terraform state file.
		logger.Info("Skipping deletion of recipe resources: Terraform state file backend does not exist.")
		return nil
	}

	// Run TF Destroy in the working directory to delete the resources deployed by the recipe
	err = initAndDestroy(ctx, tf)
	if err != nil {
		logger.Error(err, "Failed to initialize and destroy Terraform configuration")
		return err
	}

	// Delete the kubernetes secret created for terraform state file.
	err = kubernetesClient.CoreV1().
		Secrets(backends.RadiusNamespace).
		Delete(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix, metav1.DeleteOptions{})
	if err != nil {
		logger.Error(err, "Failed to delete Kubernetes secret for terraform state", "secretName", backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix)
		return fmt.Errorf("error deleting kubernetes secret for terraform state: %w", err)
	}

	return nil
}

func (e *executor) GetRecipeMetadata(ctx context.Context, options Options) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()

	terraformConfig := getEnvTerraformConfig(options)

	tf, err := Install(ctx, i, options.RootDir, terraformConfig, options.Secrets, options.LogLevel)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info("Failed to cleanup Terraform installation", "error", err.Error())
		}
	}()
	if err != nil {
		return nil, err
	}

	// Set environment variables BEFORE any module operations
	// Set environment variables for the Terraform process.
	err = e.setEnvironmentVariables(ctx, tf, options)
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
func (e *executor) setEnvironmentVariables(ctx context.Context, tf *tfexec.Terraform, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Populate envVars with the environment variables from current process
	envVars := splitEnvVar(os.Environ())

	// Remove TF_LOG to prevent conflict with terraform-exec's logging configuration
	delete(envVars, "TF_LOG")

	var envVarUpdate bool

	// Handle recipe config if present
	if options.EnvConfig != nil {
		recipeConfig := &options.EnvConfig.RecipeConfig

		if len(recipeConfig.Env.AdditionalProperties) > 0 {
			envVarUpdate = true
			logger.Info("Setting environment variables from recipe config", "count", len(recipeConfig.Env.AdditionalProperties))
			maps.Copy(envVars, recipeConfig.Env.AdditionalProperties)
		}

		if len(recipeConfig.EnvSecrets) > 0 {
			logger.Info("Extracting secret environment variables", "count", len(recipeConfig.EnvSecrets))
			for secretName, secretReference := range recipeConfig.EnvSecrets {
				logger.Info("Processing environment secret", "secretName", secretName, "source", secretReference.Source, "key", secretReference.Key)
				// Extract secret value from the secrets input
				if secretData, ok := options.Secrets[secretReference.Source]; ok {
					if secretValue, ok := secretData.Data[secretReference.Key]; ok {
						envVarUpdate = true
						envVars[secretName] = secretValue
						logger.Info("Environment secret extracted successfully", "secretName", secretName)
					} else {
						logger.Error(nil, "Missing secret key in secret store", "source", secretReference.Source, "key", secretReference.Key)
						return fmt.Errorf("missing secret key in secret store id: %s", secretReference.Source)
					}
				} else {
					logger.Error(nil, "Missing secret source", "source", secretReference.Source)
					return fmt.Errorf("missing secret source: %s", secretReference.Source)
				}
			}
		}
	}

	// Add TLS environment variables (this can run even if EnvConfig is nil)
	err := addTLSEnvironmentVariables(ctx, options, envVars)
	if err != nil {
		return fmt.Errorf("failed to add TLS environment variables: %w", err)
	}
	// If TLS settings were added, mark for update
	if options.EnvRecipe != nil && options.EnvRecipe.TLS != nil {
		envVarUpdate = true
	}

	// Add registry environment variables if provided
	if len(options.RegistryEnv) > 0 {
		envVarUpdate = true
		logger.Info("Adding registry environment variables", "count", len(options.RegistryEnv))

		// Log each registry environment variable for debugging
		for key, value := range options.RegistryEnv {
			if strings.Contains(key, "TOKEN") {
				logger.Info("Adding registry env var", "key", key, "valueLength", len(value))
			} else if strings.Contains(key, "SSL_CERT_FILE") || strings.Contains(key, "CURL_CA_BUNDLE") {
				logger.Info("Adding TLS env var", "key", key, "value", value)
			} else {
				logger.Info("Adding registry env var", "key", key, "value", value)
			}
		}

		maps.Copy(envVars, options.RegistryEnv)
	}

	// Ensure Azure auth env vars are set for providers like azapi that rely on Azure SDK defaults
	if e != nil { // executor has UCP connection to discover creds
		updated, err := e.addAzureAuthEnvironmentVariables(ctx, envVars, options)
		if err != nil {
			logger.Info("Failed to set Azure auth environment variables", "error", err.Error())
		}
		envVarUpdate = envVarUpdate || updated
	}

	// Set the environment variables for the Terraform process
	if envVarUpdate || len(envVars) > 0 {
		logger.Info("Setting environment variables for Terraform process",
			"totalCount", len(envVars),
			"hasUpdate", envVarUpdate)

		if err := tf.SetEnv(envVars); err != nil {
			logger.Error(err, "Failed to set environment variables")
			return fmt.Errorf("failed to set environment variables: %w", err)
		}

		logger.Info("Successfully set environment variables for Terraform process")
	}

	return nil
}

// addAzureAuthEnvironmentVariables sets AZURE_* and ARM_* environment variables based on UCP credentials so
// SDK-based providers (e.g., azapi) authenticate via Workload Identity or Service Principal without Azure CLI.
// Returns true if envVars was modified.
func (e *executor) addAzureAuthEnvironmentVariables(ctx context.Context, envVars map[string]string, options Options) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if e.ucpConn == nil || e.secretProvider == nil {
		return false, nil
	}

	// Build provider config using existing builder to obtain creds and subscription
	azBuilder := providers.NewAzureProvider(e.ucpConn, e.secretProvider)
	cfg, err := azBuilder.BuildConfig(ctx, options.EnvConfig)
	if err != nil {
		return false, err
	}
	if len(cfg) == 0 {
		return false, nil
	}

	changed := false
	// Map values
	getStr := func(k string) string {
		if v, ok := cfg[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	getBool := func(k string) bool {
		if v, ok := cfg[k]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
		return false
	}

	subID := getStr("subscription_id")
	clientID := getStr("client_id")
	tenantID := getStr("tenant_id")
	clientSecret := getStr("client_secret")
	useOIDC := getBool("use_oidc")
	oidcTokenPath := getStr("oidc_token_file_path")

	// Set ARM_* (Terraform providers) and AZURE_* (Azure SDK/azapi) variables
	if subID != "" {
		if envVars["ARM_SUBSCRIPTION_ID"] == "" {
			envVars["ARM_SUBSCRIPTION_ID"] = subID
			changed = true
		}
	}
	if clientID != "" {
		if envVars["ARM_CLIENT_ID"] == "" {
			envVars["ARM_CLIENT_ID"] = clientID
			changed = true
		}
		if envVars["AZURE_CLIENT_ID"] == "" {
			envVars["AZURE_CLIENT_ID"] = clientID
			changed = true
		}
	}
	if tenantID != "" {
		if envVars["ARM_TENANT_ID"] == "" {
			envVars["ARM_TENANT_ID"] = tenantID
			changed = true
		}
		if envVars["AZURE_TENANT_ID"] == "" {
			envVars["AZURE_TENANT_ID"] = tenantID
			changed = true
		}
	}
	if clientSecret != "" {
		if envVars["ARM_CLIENT_SECRET"] == "" {
			envVars["ARM_CLIENT_SECRET"] = clientSecret
			changed = true
		}
		if envVars["AZURE_CLIENT_SECRET"] == "" {
			envVars["AZURE_CLIENT_SECRET"] = clientSecret
			changed = true
		}
	}
	if useOIDC && oidcTokenPath != "" {
		if envVars["ARM_USE_OIDC"] == "" {
			envVars["ARM_USE_OIDC"] = "true"
			changed = true
		}
		if envVars["ARM_OIDC_TOKEN_FILE_PATH"] == "" {
			envVars["ARM_OIDC_TOKEN_FILE_PATH"] = oidcTokenPath
			changed = true
		}
		// For Azure SDK workload identity
		if envVars["AZURE_FEDERATED_TOKEN_FILE"] == "" {
			envVars["AZURE_FEDERATED_TOKEN_FILE"] = oidcTokenPath
			changed = true
		}
	}

	// Log key vars for troubleshooting (values redacted where sensitive)
	if changed {
		logger.Info("Set Azure auth env vars for Terraform process",
			"ARM_SUBSCRIPTION_ID_set", subID != "",
			"ARM_CLIENT_ID_set", clientID != "",
			"ARM_TENANT_ID_set", tenantID != "",
			"ARM_CLIENT_SECRET_set", clientSecret != "",
			"ARM_USE_OIDC", useOIDC,
			"ARM_OIDC_TOKEN_FILE_PATH_set", oidcTokenPath != "",
			"AZURE_CLIENT_ID_set", clientID != "",
			"AZURE_TENANT_ID_set", tenantID != "",
			"AZURE_FEDERATED_TOKEN_FILE_set", oidcTokenPath != "",
		)
	}

	return changed, nil
}

// tlsCertificatePaths holds paths to temporary certificate files
type tlsCertificatePaths struct {
	// CAPath is the path to the CA certificate file
	CAPath string
}

func addTLSEnvironmentVariables(
	ctx context.Context,
	options Options,
	envVars map[string]string,
) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// TODO(rp TLS): For now, skip honoring recipe TLS certificate settings and rely on the
	// system CA bundle mounted at /etc/ssl/certs. If this works in all environments,
	// we can remove the TLS settings from the recipe config types entirely.
	if options.EnvRecipe != nil && options.EnvRecipe.TLS != nil {
		logger.Info("Skipping recipe TLS settings; using system CA bundle instead")
	} else {
		logger.Info("No recipe TLS configuration; using system CA bundle")
	}

	// Choose system certificate locations. Prefer Debian/Ubuntu, then RHEL/CentOS.
	certFileCandidates := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
	}
	certDirCandidates := []string{
		"/etc/ssl/certs",
		"/etc/pki/tls/certs",
	}

	systemCertFile := firstExisting(certFileCandidates)
	systemCertDir := firstExistingDir(certDirCandidates)

	if systemCertFile == "" || systemCertDir == "" {
		logger.Info("System certificate paths not found; leaving TLS env unchanged",
			"file", systemCertFile, "dir", systemCertDir)
		return nil
	}

	logger.Info("Using system certificate paths", "file", systemCertFile, "dir", systemCertDir)

	// Only set defaults if not already present in envVars. Do not mutate process env.
	if val, ok := envVars["SSL_CERT_FILE"]; !ok || val == "" {
		envVars["SSL_CERT_FILE"] = systemCertFile
	}
	if val, ok := envVars["SSL_CERT_DIR"]; !ok || val == "" {
		envVars["SSL_CERT_DIR"] = systemCertDir
	}
	if val, ok := envVars["CURL_CA_BUNDLE"]; !ok || val == "" {
		envVars["CURL_CA_BUNDLE"] = systemCertFile
	}
	if val, ok := envVars["REQUESTS_CA_BUNDLE"]; !ok || val == "" {
		envVars["REQUESTS_CA_BUNDLE"] = systemCertFile
	}
	// For Git, CAINFO is a file and CAPATH is a directory.
	if val, ok := envVars["GIT_SSL_CAINFO"]; !ok || val == "" {
		envVars["GIT_SSL_CAINFO"] = systemCertFile
	}
	if val, ok := envVars["GIT_SSL_CAPATH"]; !ok || val == "" {
		envVars["GIT_SSL_CAPATH"] = systemCertDir
	}

	return nil
}

// writeTLSCertificates extracts certificate data from secrets and writes them to temporary files
func writeTLSCertificates(ctx context.Context, workingDir string, tls *recipes.TLSConfig, secrets map[string]recipes.SecretData) (*tlsCertificatePaths, error) {
	if tls == nil {
		return nil, nil
	}

	// Return nil if no certificates are configured
	if tls.CACertificate == nil {
		return nil, nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	paths := &tlsCertificatePaths{}

	// Create .tls directory in working directory
	tlsDir := filepath.Join(workingDir, ".tls")
	if err := os.MkdirAll(tlsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create TLS directory: %w", err)
	}

	// Handle CA certificate
	if tls.CACertificate != nil {
		logger.Info("Writing CA certificate", "source", tls.CACertificate.Source)
		secretData, ok := secrets[tls.CACertificate.Source]
		if !ok {
			return nil, fmt.Errorf("CA certificate secret not found: %s", tls.CACertificate.Source)
		}

		caData, ok := secretData.Data[tls.CACertificate.Key]
		if !ok {
			return nil, fmt.Errorf("CA certificate key %q not found in secret %s", tls.CACertificate.Key, tls.CACertificate.Source)
		}

		// Check if caData appears to be a file path rather than certificate content
		caDataStr := strings.TrimSpace(caData)
		if isFilePath(caDataStr) {
			logger.Error(nil, "Certificate data appears to be a file path, not certificate content",
				"path", caDataStr,
				"hint", "Use loadTextContent('./path/to/cert.crt') in Bicep to load actual certificate content")
			return nil, fmt.Errorf("certificate data appears to be a file path (%s) instead of certificate content. Use loadTextContent() in Bicep to load the actual certificate content", caDataStr)
		}

		// Validate that we have actual certificate content
		if !strings.Contains(caDataStr, "-----BEGIN CERTIFICATE-----") &&
			!strings.Contains(caDataStr, "-----BEGIN TRUSTED CERTIFICATE-----") {
			logger.Error(nil, "Certificate data does not appear to contain valid PEM certificate content",
				"dataLength", len(caDataStr),
				"dataPreview", getCertPreview(caDataStr))
			return nil, fmt.Errorf("certificate data does not contain valid PEM certificate content")
		}

		paths.CAPath = filepath.Join(tlsDir, "ca.crt")
		if err := os.WriteFile(paths.CAPath, []byte(caData), 0600); err != nil {
			return nil, fmt.Errorf("failed to write CA certificate: %w", err)
		}

		// Set comprehensive Git SSL environment variables for all Git operations
		// These environment variables ensure Git uses our custom certificate for all SSL connections
		gitSSLEnvVars := map[string]string{
			"GIT_SSL_CAINFO":     paths.CAPath, // Primary Git SSL CA certificate file
			"GIT_SSL_CAPATH":     paths.CAPath, // Git SSL CA certificate file (alternative to CAINFO)
			"SSL_CERT_FILE":      paths.CAPath, // General SSL certificate file for curl/wget
			"SSL_CERT_DIR":       tlsDir,       // General SSL certificate directory
			"CURL_CA_BUNDLE":     paths.CAPath, // Curl CA bundle file
			"REQUESTS_CA_BUNDLE": paths.CAPath, // Python requests library CA bundle
		}

		// Set in OS environment so Git processes spawned by Terraform can access them
		for envVar, envValue := range gitSSLEnvVars {
			os.Setenv(envVar, envValue)
			logger.Info("Set Git SSL environment variable globally", "var", envVar, "value", envValue)
		}

		// Configure Git SSL verbosity for debugging
		os.Setenv("GIT_CURL_VERBOSE", "1")

		// NOTE: Do NOT set GIT_SSL_CERT - that's for client certificates (private keys), not CA certificates

		logger.Info("Configured Git to use custom SSL certificate",
			"caPath", paths.CAPath,
			"GIT_SSL_CAINFO", paths.CAPath,
			"GIT_SSL_CAPATH", paths.CAPath)

		logger.Info("Successfully wrote CA certificate file with comprehensive SSL environment",
			"path", paths.CAPath,
			"size", len(caData),
			"certPreview", getCertPreview(caDataStr),
			"envVarsSet", len(gitSSLEnvVars))
	}

	return paths, nil
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

// isFilePath checks if a string appears to be a file path rather than certificate content
func isFilePath(data string) bool {
	trimmed := strings.TrimSpace(data)

	// Check if it looks like a file path
	if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "/") {
		return true
	}

	// Check if it has file extensions common for certificates
	if strings.HasSuffix(trimmed, ".crt") || strings.HasSuffix(trimmed, ".pem") ||
		strings.HasSuffix(trimmed, ".cer") || strings.HasSuffix(trimmed, ".key") {
		// But if it also contains certificate markers, it's probably content
		if strings.Contains(trimmed, "-----BEGIN") {
			return false
		}
		return true
	}

	return false
}

// getCertPreview returns a safe preview of certificate content for logging
func getCertPreview(cert string) string {
	lines := strings.Split(cert, "\n")
	if len(lines) > 0 {
		return lines[0] // Usually "-----BEGIN CERTIFICATE-----" or the first line
	}
	return "empty"
}

// firstExisting returns the first path that exists as a regular file.
func firstExisting(paths []string) string {
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// firstExistingDir returns the first path that exists as a directory.
func firstExistingDir(paths []string) string {
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

// generateConfig generates Terraform configuration with required inputs for the module, providers and backend to be initialized and applied.
func (e *executor) generateConfig(ctx context.Context, tf *tfexec.Terraform, options Options) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	workingDir := tf.WorkingDir()

	tfConfig, err := getTerraformConfig(ctx, workingDir, options)
	if err != nil {
		return "", err
	}

	loadedModule, err := downloadAndInspect(ctx, tf, options)
	if err != nil {
		return "", err
	}

	// Generate Terraform providers configuration for required providers and add it to the Terraform configuration.
	logger.Info("Adding provider config for required providers", "providers", loadedModule.RequiredProviders)
	if err := tfConfig.AddProviders(ctx, loadedModule.RequiredProviders, providers.GetUCPConfiguredTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig, options.Secrets); err != nil {
		return "", err
	}

	kubernetesClient, err := e.kubernetesClients.ClientGoClient()
	if err != nil {
		return "", fmt.Errorf("error getting kubernetes client: %w", err)
	}

	backendConfig, err := tfConfig.AddTerraformBackend(options.ResourceRecipe, backends.NewKubernetesBackend(kubernetesClient))
	if err != nil {
		return "", err
	}

	// Retrieving the secret_suffix property from backend config to use it to verify secret creation during terraform init.
	// This is only used for the backend of type kubernetes and should be moved inside an if block when we add more backends.
	var secretSuffix string
	if backendDetails, ok := backendConfig[backends.BackendKubernetes]; ok {
		backendMap := backendDetails.(map[string]any)
		if secret, ok := backendMap["secret_suffix"]; ok {
			secretSuffix = secret.(string)
		}
	}

	// Add recipe context parameter to the generated Terraform config's module parameters.
	// This should only be added if the recipe context variable is declared in the downloaded module.
	if loadedModule.ContextVarExists {
		logger.Info("Adding recipe context module result")

		// Create the recipe context object to be passed to the recipe deployment
		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return "", err
		}

		//update the recipe context with connected resources properties
		if options.ResourceRecipe != nil {
			recipectx.Resource.Connections = options.ResourceRecipe.ConnectedResourcesProperties
		}

		if err = tfConfig.AddRecipeContext(ctx, options.EnvRecipe.Name, recipectx); err != nil {
			return "", err
		}
	}
	if loadedModule.ResultOutputExists {
		if err = tfConfig.AddOutputs(options.EnvRecipe.Name); err != nil {
			return "", err
		}
	}

	// Ensure that we need to save the configuration after adding providers and recipecontext.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return "", err
	}

	// After module download and config save, force-inject credentials into any explicit provider blocks
	// in both the working directory and downloaded module sources. This ensures end-user recipes that
	// declare provider blocks always receive UCP credentials.
	if err := e.forceInjectProviderCredentials(ctx, workingDir, options); err != nil {
		logger.Info("Provider block injection encountered a non-fatal error", "error", err.Error())
		// Do not fail the deployment on injection issues; continue with best-effort
	}

	return secretSuffix, nil
}

// forceInjectProviderCredentials scans all .tf and .tf.json files under rootDir (including .terraform/modules)
// and injects UCP-provided credentials into any explicit provider blocks. This is a best-effort operation.
func (e *executor) forceInjectProviderCredentials(ctx context.Context, rootDir string, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Build provider config maps from UCP credentials
	ucpProviders := providers.GetUCPConfiguredTerraformProviders(e.ucpConn, e.secretProvider)
	providerConfigs := make(map[string]map[string]any)
	for name, builder := range ucpProviders {
		cfg, err := builder.BuildConfig(ctx, options.EnvConfig)
		if err != nil {
			logger.Info("Skipping provider due to build error", "provider", name, "error", err.Error())
			continue
		}
		if len(cfg) > 0 {
			providerConfigs[name] = cfg
		}
	}
	if len(providerConfigs) == 0 {
		logger.Info("No UCP provider configurations available to inject")
		return nil
	}

	// Walk all files and inject where applicable
	return filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tf") {
			return e.injectIntoHCLFile(ctx, path, providerConfigs)
		}
		if strings.HasSuffix(path, ".tf.json") {
			return e.injectIntoJSONFile(ctx, path, providerConfigs)
		}
		return nil
	})
}

// injectIntoHCLFile merges or appends provider credential attributes into provider blocks in an HCL file.
// It replaces existing keys' values when present; otherwise, it appends missing ones. Nested blocks are left as-is.
func (e *executor) injectIntoHCLFile(ctx context.Context, filePath string, providerConfigs map[string]map[string]any) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	content := string(contentBytes)
	modified := content

	for providerName, cfg := range providerConfigs {
		// Find all occurrences of provider "name" {
		startRe := regexp.MustCompile(fmt.Sprintf(`(?m)^[ \t]*provider\s+"%s"\s*\{`, regexp.QuoteMeta(providerName)))
		locs := startRe.FindAllStringIndex(modified, -1)
		if len(locs) == 0 {
			continue
		}
		// Process from end to start to keep indices stable after replacements
		for i := len(locs) - 1; i >= 0; i-- {
			startIdx := locs[i][0]
			// Find the opening brace position
			openBraceIdx := strings.Index(modified[startIdx:], "{")
			if openBraceIdx < 0 {
				continue
			}
			openIdx := startIdx + openBraceIdx
			closeIdx := findMatchingClosingBrace(modified, openIdx)
			if closeIdx <= openIdx || closeIdx > len(modified) {
				continue
			}

			blockStart := startIdx
			bodyStart := openIdx + 1
			bodyEnd := closeIdx

			header := modified[blockStart:bodyStart]
			body := modified[bodyStart:bodyEnd]
			footer := modified[bodyEnd:]

			// For each key in cfg, replace existing or append new
			for key, val := range cfg {
				switch v := val.(type) {
				case string:
					lineRe := regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(key) + "\\s*=.*$")
					replacement := fmt.Sprintf("  %s = \"%s\"", key, v)
					if lineRe.MatchString(body) {
						body = lineRe.ReplaceAllString(body, replacement)
					} else {
						body = strings.TrimRight(body, "\n") + "\n" + replacement + "\n"
					}
				case bool:
					lineRe := regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(key) + "\\s*=.*$")
					replacement := fmt.Sprintf("  %s = %t", key, v)
					if lineRe.MatchString(body) {
						body = lineRe.ReplaceAllString(body, replacement)
					} else {
						body = strings.TrimRight(body, "\n") + "\n" + replacement + "\n"
					}
				case int, int64, float64:
					lineRe := regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(key) + "\\s*=.*$")
					replacement := fmt.Sprintf("  %s = %v", key, v)
					if lineRe.MatchString(body) {
						body = lineRe.ReplaceAllString(body, replacement)
					} else {
						body = strings.TrimRight(body, "\n") + "\n" + replacement + "\n"
					}
				case map[string]any:
					// Add nested block if not already present
					blockRe := regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(key) + "\\s*\\{")
					if !blockRe.MatchString(body) {
						var bldr []string
						bldr = append(bldr, fmt.Sprintf("  %s {", key))
						for nk, nv := range v {
							switch nvTyped := nv.(type) {
							case string:
								bldr = append(bldr, fmt.Sprintf("    %s = \"%s\"", nk, nvTyped))
							case bool:
								bldr = append(bldr, fmt.Sprintf("    %s = %t", nk, nvTyped))
							case int, int64, float64:
								bldr = append(bldr, fmt.Sprintf("    %s = %v", nk, nv))
							}
						}
						bldr = append(bldr, "  }")
						body = strings.TrimRight(body, "\n") + "\n" + strings.Join(bldr, "\n") + "\n"
					}
				}
			}

			// Rebuild content: header + body + closing brace + rest
			modified = modified[:blockStart] + header + body + "}" + footer
		}
	}

	if modified != content {
		if err := os.WriteFile(filePath, []byte(modified), 0644); err != nil {
			logger.Info("Failed to write injected HCL file", "file", filePath, "error", err.Error())
			return nil
		}
	}
	return nil
}

// findMatchingClosingBrace finds the index of the matching '}' for a '{' at openIdx, considering nested braces and quoted strings.
func findMatchingClosingBrace(s string, openIdx int) int {
	depth := 0
	inStr := false
	escape := false
	for i := openIdx; i < len(s); i++ {
		c := s[i]
		if inStr {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
			continue
		}
	}
	return -1
}

// injectIntoJSONFile merges provider credential attributes into JSON-based Terraform files.
func (e *executor) injectIntoJSONFile(ctx context.Context, filePath string, providerConfigs map[string]map[string]any) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var tfObj map[string]any
	if err := json.Unmarshal(data, &tfObj); err != nil {
		return nil
	}
	provAny, ok := tfObj["provider"]
	if !ok {
		return nil
	}
	provMap, ok := provAny.(map[string]any)
	if !ok {
		return nil
	}
	changed := false
	for name, cfg := range providerConfigs {
		if listAny, exists := provMap[name]; exists {
			if arr, ok := listAny.([]any); ok {
				for i := range arr {
					if m, ok := arr[i].(map[string]any); ok {
						for k, v := range cfg {
							m[k] = v
						}
						arr[i] = m
						changed = true
					}
				}
			}
		}
	}
	if changed {
		out, err := json.MarshalIndent(tfObj, "", "  ")
		if err == nil {
			if err := os.WriteFile(filePath, out, 0644); err != nil {
				logger.Info("Failed to write injected JSON file", "file", filePath, "error", err.Error())
			}
		}
	}
	return nil
}

// getTerraformConfig initializes the Terraform json config with provided module source and saves it
func getTerraformConfig(ctx context.Context, workingDir string, options Options) (*config.TerraformConfig, error) {
	// Generate Terraform json config in the working directory
	// Use recipe name as a local reference to the module.
	// Modules are downloaded in a subdirectory in the working directory. Name of the module specified in the
	// configuration is used as subdirectory name under .terraform/modules directory.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	if options.EnvRecipe == nil {
		return nil, errors.New("environment recipe cannot be nil")
	}

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

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, tf *tfexec.Terraform) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform",
		"workingDir", tf.WorkingDir())

	// Check if required environment variables are set (for debugging)
	gitSSLEnvVars := []string{"GIT_SSL_CAINFO", "GIT_SSL_CAPATH", "SSL_CERT_FILE", "SSL_CERT_DIR", "CURL_CA_BUNDLE", "REQUESTS_CA_BUNDLE"}
	registryEnvVars := []string{"TF_CLI_CONFIG_FILE"}

	logger.Info("Checking Git SSL environment variables for debugging")
	for _, envKey := range gitSSLEnvVars {
		if value := os.Getenv(envKey); value != "" {
			logger.Info("Git SSL environment variable set", "key", envKey, "value", value)
		} else {
			logger.Info("Git SSL environment variable not set", "key", envKey)
		}
	}

	logger.Info("Checking registry environment variables for debugging")
	for _, envKey := range registryEnvVars {
		if value := os.Getenv(envKey); value != "" {
			logger.Info("Registry environment variable set", "key", envKey, "value", value)
		} else {
			logger.Info("Registry environment variable not set", "key", envKey)
		}
	}

	// Also check if certificate files actually exist
	if caInfo := os.Getenv("GIT_SSL_CAINFO"); caInfo != "" {
		if _, err := os.Stat(caInfo); err == nil {
			logger.Info("Certificate file exists and accessible", "path", caInfo)
		} else {
			logger.Error(err, "Certificate file not accessible", "path", caInfo)
		}
	}

	// Azure auth env diagnostics (for azurerm/azapi SDK auth)
	azureEnvVars := []string{
		"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "ARM_CLIENT_ID", "ARM_CLIENT_SECRET",
		"ARM_USE_OIDC", "ARM_OIDC_TOKEN_FILE_PATH",
		"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_FEDERATED_TOKEN_FILE",
	}
	logger.Info("Checking Azure auth environment variables for debugging")
	for _, envKey := range azureEnvVars {
		if value := os.Getenv(envKey); value != "" {
			if strings.Contains(envKey, "SECRET") {
				logger.Info("Azure auth env var set", "key", envKey, "value", "<redacted>")
			} else {
				logger.Info("Azure auth env var set", "key", envKey, "value", value)
			}
		} else {
			logger.Info("Azure auth env var not set", "key", envKey)
		}
	}
	// Verify OIDC token file presence if configured
	tokenPath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	if tokenPath == "" {
		// Fallback to common path used by azurerm provider config
		tokenPath = "/var/run/secrets/azure/tokens/azure-identity-token"
	}
	if tokenPath != "" {
		if fi, err := os.Stat(tokenPath); err == nil && fi.Mode().IsRegular() {
			logger.Info("OIDC token file present", "path", tokenPath, "size", fi.Size())
		} else if err != nil {
			logger.Info("OIDC token file missing or inaccessible", "path", tokenPath, "error", err.Error())
		}
	}

	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		logger.Error(err, "Terraform init failed during apply flow")
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return nil, fmt.Errorf("terraform init failure during apply flow: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
		[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState)})

	// Set apply options to handle locks
	applyOptions := []tfexec.ApplyOption{
		tfexec.Lock(true),
		tfexec.LockTimeout("60s"),
	}

	// Apply Terraform configuration
	logger.Info("Running Terraform apply")
	if err := tf.Apply(ctx, applyOptions...); err != nil {
		logger.Error(err, "Terraform apply failed")
		return nil, fmt.Errorf("terraform apply failure: %w", err)
	}

	// Load Terraform state to retrieve the outputs
	logger.Info("Fetching Terraform state")
	return tf.Show(ctx)
}

// initAndDestroy runs Terraform init and destroy in the provided working directory.
func initAndDestroy(ctx context.Context, tf *tfexec.Terraform) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		logger.Error(err, "Terraform init failed during destroy flow")
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return fmt.Errorf("terraform init failure during destroy flow: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime, nil)

	// Destroy Terraform configuration
	logger.Info("Running Terraform destroy")
	if err := tf.Destroy(ctx); err != nil {
		logger.Error(err, "Terraform destroy failed")
		return fmt.Errorf("terraform destroy failure: %w", err)
	}

	return nil
}
