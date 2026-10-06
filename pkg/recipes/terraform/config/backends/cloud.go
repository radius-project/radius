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

package backends

import (
	"fmt"
	"os"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
)

const (
	BackendS3      = "s3"
	BackendAzureRM = "azurerm"

	// backendSessionPrefix identifies Radius backend state access in AWS CloudTrail. The state key hash is
	// appended so that a state read or write can be traced to the Radius resource that caused it. The
	// result stays within the 64 character AWS session name limit.
	backendSessionPrefix = "radius-tf-backend-"
)

// CloudBackend renders a built-in cloud backend. Terraform owns state access and locking.
type CloudBackend struct {
	Settings *datamodel.TerraformBackend

	// Auth carries non-secret backend authentication. Secret-bearing credential modes are delivered
	// through the process environment instead, so Auth is empty for them.
	Auth CloudBackendAuth
}

// CloudBackendAuth configures backend authentication that contains no secret and can therefore be
// rendered into the generated backend block. Keeping it out of the process environment leaves recipe
// provider authentication untouched.
type CloudBackendAuth struct {
	// AWSRoleARN selects AWS IRSA web identity authentication for the s3 backend.
	AWSRoleARN string

	// AWSProfile is the profile defined in AWSSharedConfigFile. It is rendered into the backend
	// block so that a profile selected by the execution environment cannot be used instead.
	AWSProfile string

	// AWSSharedConfigFile is a Radius-generated AWS shared configuration file containing only
	// AWSProfile. Rendering it, alongside an empty shared credentials file, keeps shared
	// configuration that the execution environment points at — which can carry endpoint overrides
	// that redirect state traffic and the IRSA token exchange — out of backend authentication,
	// while leaving the process environment the recipe's providers read untouched.
	AWSSharedConfigFile string

	// AzureClientID selects Azure Workload Identity authentication for the azurerm backend.
	AzureClientID string

	// AzureTenantID is the tenant of AzureClientID.
	AzureTenantID string

	// AzureEnvironment records the value of ARM_ENVIRONMENT in the execution environment, which is
	// how the azurerm backend selects its Azure cloud. It is not rendered into the backend block;
	// Terraform reads the variable itself. It is carried here so that state cleanup resolves the
	// same storage endpoint and token authority Terraform used, rather than assuming public Azure.
	// Empty means the default, public Azure.
	AzureEnvironment string
}

// BuildBackend renders location, locking and non-secret authentication options. Secrets are never
// rendered into the backend block.
func (b CloudBackend) BuildBackend(resource *recipes.ResourceMetadata) (map[string]any, error) {
	if b.Settings == nil || resource == nil {
		return nil, fmt.Errorf("cloud backend settings and resource metadata are required")
	}
	if err := b.Settings.Validate(); err != nil {
		return nil, err
	}
	// Reuse the current resource identity, not Kubernetes's legacy-secret discovery.
	hash, err := generateSecretSuffix(resource)
	if err != nil {
		return nil, err
	}
	key := b.Settings.KeyPrefix + "/" + hash + ".tfstate"
	if b.Settings.Type == BackendS3 {
		config := map[string]any{
			"bucket":       b.Settings.Bucket,
			"region":       b.Settings.Region,
			"key":          key,
			"use_lockfile": true,
		}
		// Mirrors the AWS provider's IRSA configuration so backend and provider authentication cannot drift.
		if b.Auth.AWSRoleARN != "" {
			config["assume_role_with_web_identity"] = map[string]any{
				"role_arn":                b.Auth.AWSRoleARN,
				"session_name":            backendSessionPrefix + hash,
				"web_identity_token_file": providers.AWSIRSATokenFilePath,
			}
		}
		// Bind the backend to Radius-generated shared configuration, so that configuration the
		// execution environment selects cannot redirect state traffic or the token exchange.
		//
		// Naming a profile is what neutralizes AWS_PROFILE, which this mode leaves in place for the
		// recipe's providers. It also makes Terraform warn about a "configuration conflict" when
		// static AWS keys are present in that environment (aws-sdk-go-base credentials.go,
		// getCredentialsProvider). The warning is inaccurate here, because the rendered web identity
		// replaces resolved credentials outright, and it does not fail the execution.
		if b.Auth.AWSSharedConfigFile != "" {
			config["profile"] = b.Auth.AWSProfile
			config["shared_config_files"] = []string{b.Auth.AWSSharedConfigFile}
			config["shared_credentials_files"] = []string{os.DevNull}
		}
		return map[string]any{BackendS3: config}, nil
	}
	config := map[string]any{
		"storage_account_name": b.Settings.StorageAccountName,
		"container_name":       b.Settings.ContainerName,
		"key":                  key,
		"use_azuread_auth":     true,
	}
	if b.Auth.AzureClientID != "" {
		config["use_oidc"] = true
		config["client_id"] = b.Auth.AzureClientID
		config["tenant_id"] = b.Auth.AzureTenantID
		config["oidc_token_file_path"] = providers.AzureOIDCTokenFilePath
	}
	return map[string]any{BackendAzureRM: config}, nil
}

// StateKey returns the state object key rendered into the given cloud backend configuration.
func StateKey(backendConfig map[string]any) (backendType string, key string) {
	for _, name := range []string{BackendS3, BackendAzureRM} {
		config, ok := backendConfig[name].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := config["key"].(string); ok {
			return name, value
		}
	}
	return "", ""
}
