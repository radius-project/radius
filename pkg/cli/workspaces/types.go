// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workspaces

import "github.com/project-radius/radius/pkg/cli/config"

// Workspace represents configuration for the rad CLI.
//
// Workspaces may:
//
// - be stored in per-user config (~/.rad/config.yaml) OR
//
// - be stored in the user's working directory `$pwd/.rad/rad.yaml` OR
//
// - may represent the rad CLI's fallback configuration when no configuration is present
type Workspace struct {
	// Source indicates how the workspace was loaded.
	Source Source `json:"-" mapstructure:"-" yaml:"-"`

	// Directory config contains per-directory overrides and settings that affect the behavior of `rad`.
	// This is not stored in the `~/.rad/config.yaml`.
	DirectoryConfig config.DirectoryConfig `json:"-" mapstructure:"-" yaml:"-"`

	// Name is the name of the workspace. The name is not stored as part of the workspace entry but is populated
	// by the configuration APIs in this package.
	//
	// Will be set if the Source == SourceUserConfig, otherwise will be empty.
	Name string `json:"-" mapstructure:"-" yaml:"-"`

	// Connection represents the connection to the workspace. The details required by the connection are different
	// depending on the kind of connection. For example a Kubernetes connection requires a valid kubeconfig context
	// entry and a namespace.
	Connection map[string]any `json:"connection" mapstructure:"connection" yaml:"connection" validate:"required"`

	// Environment represents the default environment used for deployments of applications. This field is optional.
	Environment string `json:"environment,omitempty" mapstructure:"environment" yaml:"environment,omitempty"`

	// Registry represent a container registry to use for container image push/pull operations. This field is optional.
	Registry *Registry `json:"registry,omitempty" mapstructure:"registry" yaml:"registry,omitempty"`

	// Scope represents the default scope used for deployments of Radius resources. This field is optional.
	Scope string `json:"scope,omitempty" mapstructure:"scope" yaml:"scope,omitempty"`

	// DefaultApplication represents the default application used for deployments and management commands. This field is optional.
	DefaultApplication string `json:"defaultApplication,omitempty" mapstructure:"defaultApplication" yaml:"defaultApplication,omitempty"`

	// ProviderConfig represents the configuration for IAC providers used during deployment. This field is optional and not written to disk.
	ProviderConfig ProviderConfig `json:"-"  yaml:"-"`
}

// IsNamedWorkspace returns true for workspaces stored in per-user configuration. These workspaces have names that can
// be referenced in commands with the `--workspace` flag.
func (w Workspace) IsNamedWorkspace() bool {
	return w.Source == SourceUserConfig
}

// IsEditableWorkspace returns true for workspaces stored in per-user or directory-based configuration. These workspaces
// have configuration files and thus can have their settings updated.
func (w Workspace) IsEditableWorkspace() bool {
	return w.Source != SourceFallback
}

// Source specifies how a workspace was loaded.
type Source string

const (
	// SourceFallback indicates that the workspace was not loaded from config, and is using default settings.
	SourceFallback = "fallback"

	// SourceLocalDirectory indicates that the workspace was loaded from the users working directory.
	SourceLocalDirectory = "localdirectory"

	// SourceUserConfig indicates that the workspace was loaded from per-user config.
	SourceUserConfig = "userconfig"
)

// ProviderConfig represents the configuration for IAC providers used during deployment.
type ProviderConfig struct {
	// Azure represents the configuration for the Azure IAC provider used during deployment. This field is optional.
	Azure *AzureProvider
	AWS   *AWSProvider
}

type AzureProvider struct {
	SubscriptionID string
	ResourceGroup  string
}

type AWSProvider struct {
	Region    string
	AccountId string
}

// Registry represent the configuration for a container registry.
type Registry struct {
	// PushEndpoint is the endpoint used for push commands. For a local container registry this hostname
	// is expected to be accessible from the host machine.
	PushEndpoint string `json:"pushEndpoint,omitempty" mapstructure:"pushEndpoint" validate:"required" yaml:"pushEndpoint,omitempty"`

	// PullEndpoint is the endpoing used to pull by the runtime. For a local container registry this hostname
	// is expected to be accessible by the runtime. Can be the same as PushEndpoint if the registry has a routable
	// address.
	PullEndpoint string `json:"pullEndpoint,omitempty" mapstructure:"pullEndpoint" validate:"required" yaml:"pullEndpoint,omitempty"`
}
