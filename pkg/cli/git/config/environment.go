// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

// Package config provides configuration loading and validation for Git workspace mode.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/radius-project/radius/pkg/cli/git"
)

// Environment represents a deployment target configuration loaded from .env files.
type Environment struct {
	Name               string
	FilePath           string
	AWS                *AWSConfig
	Azure              *AzureConfig
	Kubernetes         *KubernetesConfig
	Recipes            []string
	TerraformCLIConfig string
	TerraformBackend   string
}

// AWSConfig contains AWS-specific deployment configuration.
type AWSConfig struct {
	AccountID string
	Region    string
}

// AzureConfig contains Azure-specific deployment configuration.
type AzureConfig struct {
	SubscriptionID string
	ResourceGroup  string
}

// KubernetesConfig contains Kubernetes runtime configuration.
type KubernetesConfig struct {
	Context   string
	Namespace string
}

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|secret|key|token|credential)=.+`),
	regexp.MustCompile(`(?i)^(aws_secret_access_key|aws_session_token)=`),
	regexp.MustCompile(`(?i)^(azure_client_secret|azure_tenant_id.*secret)=`),
}

// LoadEnvironment loads an environment configuration from a .env file.
func LoadEnvironment(envFilePath string) (*Environment, error) {
	absPath, err := filepath.Abs(envFilePath)
	if err != nil {
		return nil, git.NewValidationError("invalid environment file path", err.Error())
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, git.NewValidationError("environment file not found", absPath)
	}

	envMap, err := godotenv.Read(absPath)
	if err != nil {
		return nil, git.NewValidationError("failed to parse environment file", err.Error())
	}

	env := &Environment{
		Name:     deriveEnvironmentName(absPath),
		FilePath: absPath,
	}

	if awsAccountID, ok := envMap["AWS_ACCOUNT_ID"]; ok {
		env.AWS = &AWSConfig{
			AccountID: awsAccountID,
			Region:    envMap["AWS_REGION"],
		}
	}

	if azureSubscription, ok := envMap["AZURE_SUBSCRIPTION_ID"]; ok {
		env.Azure = &AzureConfig{
			SubscriptionID: azureSubscription,
			ResourceGroup:  envMap["AZURE_RESOURCE_GROUP"],
		}
	}

	if k8sContext, ok := envMap["KUBERNETES_CONTEXT"]; ok {
		env.Kubernetes = &KubernetesConfig{
			Context:   k8sContext,
			Namespace: envMap["KUBERNETES_NAMESPACE"],
		}
	}

	if recipePaths, ok := envMap["RECIPES"]; ok && recipePaths != "" {
		env.Recipes = parseCommaSeparated(recipePaths)
	}

	env.TerraformCLIConfig = envMap["TF_CLI_CONFIG_FILE"]
	env.TerraformBackend = envMap["TF_BACKEND_CONFIG"]

	return env, nil
}

// Validate checks that the environment configuration is complete and valid.
func (e *Environment) Validate() error {
	var errors []string

	if len(e.Recipes) == 0 {
		errors = append(errors, "RECIPES environment variable is required")
	}

	for _, recipePath := range e.Recipes {
		resolvedPath := e.resolvePath(recipePath)
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("recipe file not found: %s", recipePath))
		}
	}

	if e.AWS != nil {
		if e.AWS.AccountID == "" {
			errors = append(errors, "AWS_ACCOUNT_ID is required when using AWS")
		}
		if e.AWS.Region == "" {
			errors = append(errors, "AWS_REGION is required when using AWS")
		}
	}

	if e.Azure != nil {
		if e.Azure.SubscriptionID == "" {
			errors = append(errors, "AZURE_SUBSCRIPTION_ID is required when using Azure")
		}
		if e.Azure.ResourceGroup == "" {
			errors = append(errors, "AZURE_RESOURCE_GROUP is required when using Azure")
		}
	}

	if e.Kubernetes != nil {
		if e.Kubernetes.Context == "" {
			errors = append(errors, "KUBERNETES_CONTEXT is required when using Kubernetes")
		}
		if e.Kubernetes.Namespace == "" {
			errors = append(errors, "KUBERNETES_NAMESPACE is required when using Kubernetes")
		}
	}

	if len(errors) > 0 {
		return git.NewValidationError(
			fmt.Sprintf("environment '%s' validation failed", e.Name),
			strings.Join(errors, "; "),
		)
	}

	return nil
}

// CheckCredentialPatterns checks the .env file for potential credential values.
func (e *Environment) CheckCredentialPatterns() []string {
	var warnings []string

	content, err := os.ReadFile(e.FilePath)
	if err != nil {
		return warnings
	}

	lines := strings.Split(string(content), "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		for _, pattern := range credentialPatterns {
			if pattern.MatchString(line) {
				warnings = append(warnings, fmt.Sprintf("line %d may contain credentials", lineNum+1))
				break
			}
		}
	}

	return warnings
}

// HasCloudConfig returns true if any cloud platform is configured.
func (e *Environment) HasCloudConfig() bool {
	return e.AWS != nil || e.Azure != nil
}

// HasKubernetesConfig returns true if Kubernetes is configured.
func (e *Environment) HasKubernetesConfig() bool {
	return e.Kubernetes != nil
}

func deriveEnvironmentName(filePath string) string {
	baseName := filepath.Base(filePath)

	if baseName == ".env" {
		return "default"
	}

	if strings.HasPrefix(baseName, ".env.") {
		return strings.TrimPrefix(baseName, ".env.")
	}

	return strings.TrimSuffix(baseName, filepath.Ext(baseName))
}

func parseCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (e *Environment) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	envDir := filepath.Dir(e.FilePath)
	return filepath.Join(envDir, path)
}

// DiscoverEnvironments finds all .env files in the specified directory.
func DiscoverEnvironments(dir string) ([]string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var envFiles []string

	defaultEnv := filepath.Join(absDir, ".env")
	if _, err := os.Stat(defaultEnv); err == nil {
		envFiles = append(envFiles, defaultEnv)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return envFiles, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".env.") && !strings.HasSuffix(name, ".example") {
			envFiles = append(envFiles, filepath.Join(absDir, name))
		}
	}

	return envFiles, nil
}
