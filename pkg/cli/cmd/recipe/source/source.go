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

package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/discovery/config"
	"github.com/spf13/cobra"
)

const (
	flagSourceName   = "name"
	flagSourceType   = "type"
	flagSourceURL    = "url"
	flagPriority     = "priority"
	flagAuthType     = "auth-type"
	flagAuthToken    = "auth-token"
	flagAuthTokenEnv = "auth-token-env"
	flagAuthUsername = "auth-username"
	flagAuthPassword = "auth-password"
	flagConfigPath   = "config"
)

// NewCommand creates an instance of the `rad recipe source add` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a recipe source for discovery",
		Long: `Add a new recipe source for automatic discovery.

Recipe sources are locations where Radius will search for IaC modules (Terraform, Bicep)
that can be used as recipes to provision infrastructure dependencies.

Supported source types:
- avm: Azure Verified Modules from the Bicep registry
- terraform: Terraform Registry modules
- git: Git repository containing IaC modules
- local: Local directory with IaC modules

Authentication can be configured using:
- Token: Access token for private repositories
- Environment variable: Store token in env var for security
- Basic auth: Username and password
`,
		Example: `
# Add Azure Verified Modules source
rad recipe source add --name avm --type avm --url mcr.microsoft.com/bicep/avm

# Add a Terraform Registry source
rad recipe source add --name hashicorp --type terraform --url registry.terraform.io

# Add a private Git repository with token auth
rad recipe source add --name internal --type git --url github.com/myorg/recipes \
  --auth-type token --auth-token-env GITHUB_TOKEN

# Add a local directory source
rad recipe source add --name local-recipes --type local --url ./recipes

# Add a source with priority (lower = higher priority)
rad recipe source add --name primary --type avm --url mcr.microsoft.com/bicep/avm --priority 1
`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP(flagSourceName, "n", "", "Source name (required)")
	cmd.Flags().StringP(flagSourceType, "t", "", "Source type: avm, terraform, git, local (required)")
	cmd.Flags().StringP(flagSourceURL, "u", "", "Source URL or path (required)")
	cmd.Flags().Int(flagPriority, 10, "Priority (lower = higher priority)")
	cmd.Flags().String(flagAuthType, "", "Authentication type: token, env, basic")
	cmd.Flags().String(flagAuthToken, "", "Access token (use --auth-token-env for security)")
	cmd.Flags().String(flagAuthTokenEnv, "", "Environment variable containing access token")
	cmd.Flags().String(flagAuthUsername, "", "Username for basic auth")
	cmd.Flags().String(flagAuthPassword, "", "Password for basic auth")
	cmd.Flags().String(flagConfigPath, "", "Path to config file (default: .rad/config.yaml)")

	_ = cmd.MarkFlagRequired(flagSourceName)
	_ = cmd.MarkFlagRequired(flagSourceType)
	_ = cmd.MarkFlagRequired(flagSourceURL)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad recipe source add` command.
type Runner struct {
	Output output.Interface

	SourceName   string
	SourceType   string
	SourceURL    string
	Priority     int
	AuthType     string
	AuthToken    string
	AuthTokenEnv string
	AuthUsername string
	AuthPassword string
	ConfigPath   string
}

// NewRunner creates an instance of the runner for the `rad recipe source add` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate implements the framework.Runner interface.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Get flags
	r.SourceName, _ = cmd.Flags().GetString(flagSourceName)
	r.SourceType, _ = cmd.Flags().GetString(flagSourceType)
	r.SourceURL, _ = cmd.Flags().GetString(flagSourceURL)
	r.Priority, _ = cmd.Flags().GetInt(flagPriority)
	r.AuthType, _ = cmd.Flags().GetString(flagAuthType)
	r.AuthToken, _ = cmd.Flags().GetString(flagAuthToken)
	r.AuthTokenEnv, _ = cmd.Flags().GetString(flagAuthTokenEnv)
	r.AuthUsername, _ = cmd.Flags().GetString(flagAuthUsername)
	r.AuthPassword, _ = cmd.Flags().GetString(flagAuthPassword)
	r.ConfigPath, _ = cmd.Flags().GetString(flagConfigPath)

	// Validate source name
	if r.SourceName == "" {
		return fmt.Errorf("source name is required")
	}

	// Validate source type
	validTypes := []string{"avm", "terraform", "git", "local"}
	valid := false
	for _, t := range validTypes {
		if r.SourceType == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid source type %q: must be one of: avm, terraform, git, local", r.SourceType)
	}

	// Validate source URL
	if r.SourceURL == "" {
		return fmt.Errorf("source URL is required")
	}

	// Validate auth configuration
	if r.AuthType != "" {
		switch r.AuthType {
		case "token":
			if r.AuthToken == "" && r.AuthTokenEnv == "" {
				return fmt.Errorf("auth type 'token' requires --auth-token or --auth-token-env")
			}
		case "env":
			if r.AuthTokenEnv == "" {
				return fmt.Errorf("auth type 'env' requires --auth-token-env")
			}
		case "basic":
			if r.AuthUsername == "" || r.AuthPassword == "" {
				return fmt.Errorf("auth type 'basic' requires --auth-username and --auth-password")
			}
		default:
			return fmt.Errorf("invalid auth type %q: must be one of: token, env, basic", r.AuthType)
		}
	}

	// Set default config path
	if r.ConfigPath == "" {
		r.ConfigPath = filepath.Join(".rad", "config.yaml")
	}

	return nil
}

// Run implements the framework.Runner interface.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("")
	r.Output.LogInfo("Adding recipe source: %s", r.SourceName)

	// Load existing config
	cfg, err := config.LoadSourcesConfig()
	if err != nil {
		// Create new config if it doesn't exist
		cfg = &config.SourcesConfig{
			Version: "1.0",
			Sources: []config.RecipeSourceConfig{},
		}
	}

	// Build auth config
	var authConfig *config.AuthConfig
	if r.AuthType != "" {
		authConfig = &config.AuthConfig{
			Type:        r.AuthType,
			Token:       r.AuthToken,
			TokenEnvVar: r.AuthTokenEnv,
			Username:    r.AuthUsername,
			Password:    r.AuthPassword,
		}
	}

	// Create source config
	source := config.RecipeSourceConfig{
		Name:     r.SourceName,
		Type:     r.SourceType,
		URL:      r.SourceURL,
		Priority: r.Priority,
		Auth:     authConfig,
	}

	// Add source
	if err := cfg.AddSource(source); err != nil {
		return fmt.Errorf("adding source: %w", err)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(r.ConfigPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Save config
	if err := config.SaveSourcesConfig(cfg, r.ConfigPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✓ Recipe source added successfully")
	r.Output.LogInfo("")
	r.Output.LogInfo("Source Details:")
	r.Output.LogInfo("  Name:     %s", r.SourceName)
	r.Output.LogInfo("  Type:     %s", r.SourceType)
	r.Output.LogInfo("  URL:      %s", r.SourceURL)
	r.Output.LogInfo("  Priority: %d", r.Priority)
	if r.AuthType != "" {
		r.Output.LogInfo("  Auth:     %s", r.AuthType)
	}
	r.Output.LogInfo("")
	r.Output.LogInfo("Config saved to: %s", r.ConfigPath)
	r.Output.LogInfo("")
	r.Output.LogInfo("Use 'rad recipe source list' to view all configured sources.")
	r.Output.LogInfo("")

	return nil
}
