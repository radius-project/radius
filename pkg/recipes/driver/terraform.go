// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/util"
)

const (
	installDirRoot = "/terraform/install"
	workingDirRoot = "/terraform/exec"

	defaultCredential = "default"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver(ucpConn sdk.Connection) Driver {
	return &terraformDriver{UcpConn: ucpConn}
}

type terraformDriver struct {
	UcpConn sdk.Connection
}

type TerraformConfig struct {
	Terraform TerraformDefinition    `json:"terraform"`
	Provider  map[string]interface{} `json:"provider"`
	Module    map[string]ModuleData  `json:"module"`
}

type TerraformDefinition struct {
	RequiredProviders map[string]interface{} `json:"required_providers"`
	RequiredVersion   string                 `json:"required_version"`
}

type ModuleData struct {
	Source     string                 `json:"source"`
	Version    string                 `json:"version"`
	Parameters map[string]interface{} `json:",inline"`
}

func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.Metadata, definition recipes.Definition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))

	// Create Terraform installation directory
	installDir := filepath.Join(installDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform install directory: %q", installDir))
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for terraform installation for resource %q: %w", recipe.ResourceID, err)
	}
	defer func() {
		err := os.RemoveAll(installDir)
		if err != nil {
			logger.Error(err, "Failed to remove Terraform installation directory")
		}
	}()

	// Install Terraform
	// Using latest version, revisit this if we want to use a specific version.
	installer := &releases.LatestVersion{
		Product:    product.Terraform,
		InstallDir: installDir,
	}
	// We should look into checking if an exsiting installation of Terraform is available.
	// For initial iteration we will always install Terraform. Optimizations can be made later.
	execPath, err := installer.Install(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to install terraform for resource %q: %w", recipe.ResourceID, err)
	}
	// TODO check if anything else is needed to clean up Terraform installation.
	defer func() {
		err := installer.Remove(ctx)
		if err != nil {
			logger.Error(err, "Failed to remove Terraform installation")
		}
	}()

	// Create working directory for Terraform execution
	workingDir := filepath.Join(workingDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err = os.MkdirAll(workingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create working directory for terraform execution for Radius resource %q. %w", recipe.ResourceID, err)
	}
	defer func() {
		err := os.RemoveAll(workingDir)
		if err != nil {
			logger.Error(err, "Failed to remove Terraform working directory")
		}
	}()

	// Generate Terraform json config in the working directory
	// TODO scope should be passed based on the provider needed for the recipe. Hardcoding to Azure for now.
	if err = d.generateJsonConfig(ctx, workingDir, recipe.Name, definition.TemplatePath, configuration.Providers); err != nil {
		return nil, err
	}

	if err = d.initAndApply(ctx, recipe.ResourceID, workingDir, execPath); err != nil {
		return nil, err
	}
	logger.Info("Successfully deployed Terraform recipe")

	return nil, nil
}

// Runs Terraform init and apply in the provided working directory.
func (d *terraformDriver) initAndApply(ctx context.Context, resourceID, workingDir, execPath string) error {
	logger := logr.FromContextOrDiscard(ctx)

	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return err
	}

	// Initialize Terraform
	if err := tf.Init(ctx); err != nil {
		return fmt.Errorf("terraform init failure. Radius resource %q. %w", resourceID, err)
	}
	logger.Info("Terraform init completed")

	// Apply Terraform configuration
	if err := tf.Apply(ctx); err != nil {
		return fmt.Errorf("terraform apply failure. Radius resource %q. %w", resourceID, err)
	}
	logger.Info("Terraform Apply completed")

	tfState, err := tf.Show(ctx)
	if err != nil {
		return err
	}
	logger.Info("Terraform show completed")
	for k, m := range tfState.Values.Outputs {
		logger.Info(fmt.Sprintf("Terraform output: %s", k))
		logger.Info(fmt.Sprintf("Terraform output: %s", m.Value))
	}

	return nil
}

// Generate Terraform configuration in JSON format for required providers and modules
// and write it to a file in the specified working directory.
// This JSON configuration is needed to initialize and apply Terraform modules.
// See https://www.terraform.io/docs/language/syntax/json.html for more information
// on the JSON syntax for Terraform configuration.
// templatePath is the path to the Terraform module source, e.g. "Azure/cosmosdb/azurerm".
func (d *terraformDriver) generateJsonConfig(ctx context.Context, workingDir, recipeName, templatePath string, providers datamodel.Providers) error {
	logger := logr.FromContextOrDiscard(ctx)

	// TODO setting provider data for both AWS and Azure until we implement a way to pass this in.
	// This should be set based on the provider needed by the recipe/source module and not just Azure/AWS providers.
	azureProviderConfig, err := d.buildAzureProviderConfig(ctx, providers.Azure.Scope)
	if err != nil {
		return err
	}
	awsProviderConfig, err := d.buildAWSProviderConfig(ctx, providers.AWS.Scope)
	if err != nil {
		return err
	}

	_, resourceGroup, err := parseAzureScope(providers.Azure.Scope)
	if err != nil {
		return err
	}

	tfConfig := TerraformConfig{
		Terraform: TerraformDefinition{
			RequiredProviders: map[string]interface{}{
				"azurerm": map[string]interface{}{
					"source":  "hashicorp/azurerm",
					"version": "~> 3.0.2",
				},
				"aws": map[string]interface{}{
					"source":  "hashicorp/aws",
					"version": "~> 4.0",
				},
			},
			RequiredVersion: ">= 1.1.0",
		},
		Provider: map[string]interface{}{
			"azurerm": azureProviderConfig,
			"aws":     awsProviderConfig,
		},
		Module: map[string]ModuleData{
			"cosmosdb": {
				Source:     templatePath,
				Version:    "1.0.0", // TODO determine how to pass this in.
				Parameters: generateInputParameters(resourceGroup),
			},
		},
	}

	// Convert the Terraform config to JSON
	jsonData, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	logger.Info(fmt.Sprintf("Generated JSON!!! %s", jsonData))

	// Write the JSON data to a file in the working directory
	// JSON configuration syntax for Terraform requires the file to be named with .tf.json suffix.
	// https://developer.hashicorp.com/terraform/language/syntax/json
	configFilePath := fmt.Sprintf("%s/main.tf.json", workingDir)
	file, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}

// Returns the Terraform provider configuration for Azure provider.
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
func (d *terraformDriver) buildAzureProviderConfig(ctx context.Context, scope string) (map[string]interface{}, error) {
	logger := logr.FromContextOrDiscard(ctx)

	subscriptionID, _, err := parseAzureScope(scope)
	if err != nil {
		return nil, err
	}

	credentials, err := d.fetchAzureCredentials()
	if err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("Fetched Azure credentials for client id %q", credentials.ClientID))

	azureConfig := map[string]interface{}{
		"subscription_id": subscriptionID,
		"client_id":       credentials.ClientID,
		"client_secret":   credentials.ClientSecret,
		"tenant_id":       credentials.TenantID,
	}

	return azureConfig, nil
}

// Returns the Terraform provider configuration for AWS provider.
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs
func (d *terraformDriver) buildAWSProviderConfig(ctx context.Context, scope string) (map[string]interface{}, error) {
	logger := logr.FromContextOrDiscard(ctx)
	if scope == "" {
		return map[string]interface{}{}, nil
	}

	account, region, err := parseAWSScope(scope)
	if err != nil {
		return nil, err
	}

	credentials, err := d.fetchAWSCredentials()
	if err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("Fetched AWS credentials for client id %q", credentials.AccessKeyID))

	awsConfig := map[string]interface{}{
		"region":              region,
		"allowed_account_ids": []string{account},
		"access_key":          credentials.AccessKeyID,
		"secret_key":          credentials.SecretAccessKey,
	}

	return awsConfig, nil
}

func parseAzureScope(scope string) (subscriptionID string, resourceGroup string, err error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", "", fmt.Errorf("error parsing Azure scope: %w", err)
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if segment.Type == resources.SubscriptionsSegment {
			subscriptionID = segment.Name
		}

		if segment.Type == resources.ResourceGroupsSegment {
			resourceGroup = segment.Name
		}
	}

	return
}

func parseAWSScope(scope string) (account string, region string, err error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", "", fmt.Errorf("error parsing AWS scope: %w", err)
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if segment.Type == resources.AccountsSegment {
			account = segment.Name
		}

		if segment.Type == resources.RegionsSegment {
			region = segment.Name
		}
	}
	return
}

func (d *terraformDriver) fetchAzureCredentials() (*credentials.AzureCredential, error) {
	secretProvider := provider.NewSecretProvider(provider.SecretProviderOptions{
		Provider: provider.TypeKubernetesSecret,
	})
	azureCredentialProvider, err := credentials.NewAzureCredentialProvider(secretProvider, d.UcpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		return nil, fmt.Errorf("error creating Azure credential provider: %w", err)
	}

	credentials, err := azureCredentialProvider.Fetch(context.Background(), credentials.AzureCloud, defaultCredential)
	if err != nil {
		return nil, fmt.Errorf("error fetching Azure credentials: %w", err)
	}

	if credentials.ClientID == "" || credentials.ClientSecret == "" || credentials.TenantID == "" {
		return nil, errors.New("credentials are required to create Azure resources through Recipe. Use `rad credential register azure` to register Azure credentials")
	}

	return credentials, nil
}

func (d *terraformDriver) fetchAWSCredentials() (*credentials.AWSCredential, error) {
	secretProvider := provider.NewSecretProvider(provider.SecretProviderOptions{
		Provider: provider.TypeKubernetesSecret,
	})
	awsCredentialProvider, err := credentials.NewAWSCredentialProvider(secretProvider, d.UcpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		return nil, fmt.Errorf("error creating AWS credential provider: %w", err)
	}

	credentials, err := awsCredentialProvider.Fetch(context.Background(), credentials.AWSPublic, defaultCredential)
	if err != nil {
		return nil, fmt.Errorf("error fetching AWS credentials: %w", err)
	}

	if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		return nil, errors.New("credentials are required to create AWS resources through Recipe. Use `rad credential register aws` to register AWS credentials")
	}

	return credentials, nil
}

// TODO hardcoded for testing. Pending integration with parameters provided by the operator and developer
func generateInputParameters(resourceGroup string) map[string]interface{} {
	parameters := map[string]interface{}{
		"cosmos_account_name": "tf-test",
		"cosmos_api":          "mongo",
		"mongo_dbs": map[string]interface{}{
			"one": map[string]interface{}{
				"db_name":           "tf-test-db",
				"db_throughput":     400,
				"db_max_throughput": nil,
			},
		},
		"mongo_db_collections": map[string]interface{}{
			"one": map[string]interface{}{
				"collection_name":           "tf-test-collection",
				"db_name":                   "dbautoscale",
				"default_ttl_seconds":       "2592000",
				"shard_key":                 "MyShardKey",
				"collection_throughout":     400,
				"collection_max_throughput": nil,
				"analytical_storage_ttl":    nil,
				"indexes": map[string]interface{}{
					"indexone": map[string]interface{}{
						"mongo_index_keys":   []interface{}{"_id"},
						"mongo_index_unique": true,
					},
				},
			},
		},
		"location":            "westus2",
		"resource_group_name": resourceGroup,
	}

	return parameters
}
