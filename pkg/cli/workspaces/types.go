// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workspaces

// Workspace represents a single workspace entry in config.
type Workspace struct {
	// Name is the name of the workspace. The name is not stored as part of the workspace entry but is populated
	// by the configuration APIs in this package.
	Name string `json:"-" mapstructure:"-" yaml:"-"`

	// Connection represents the connection to the workspace. The details required by the connection are different
	// depending on the kind of connection. For example a Kubernetes connection requires a valid kubeconfig context
	// entry and a namespace.
	Connection map[string]interface{} `json:"connection" mapstructure:"connection" yaml:"connection" validate:"required"`

	// Environment represents the default environment used for deployments of applications. This field is optional.
	Environment string `json:"environment,omitempty" mapstructure:"environment" yaml:"environment,omitempty"`

	// Registry represent a container registry to use for container image push/pull operations. This field is optional.
	Registry *Registry `json:"registry,omitempty" mapstructure:"registry" yaml:"registry,omitempty"`

	// Scope represents the default scope used for deployments of Radius resources. This field is optional.
	Scope string `json:"scope,omitempty" mapstructure:"scope" yaml:"scope,omitempty"`

	// DefaultApplication represents the default application used for deployments and management commands. This field is optional.
	DefaultApplication string `json:"defaultApplication,omitempty" mapstructure:"defaultApplication" yaml:"defaultApplication,omitempty"`

	// ProviderConfig represents the configuration for IAC providers used during deployment. This field is optional.
	ProviderConfig ProviderConfig `json:"providerConfig,omitempty" mapstructure:"providerConfig" yaml:"providerConfig,omitempty" validate:"dive"`
}

// ProviderConfig represents the configuration for IAC providers used during deployment.
type ProviderConfig struct {
	// Azure represents the configuration for the Azure IAC provider used during deployment. This field is optional.
	Azure *Provider `json:"azure,omitempty" mapstructure:"azure" yaml:"azure,omitempty"`
}

type Provider struct {
	SubscriptionID string
	ResourceGroup  string
}

// Registry represent the configuration for a container registry.
type Registry struct {
	// PushEndpoint is the endpoint used for push commands. For a local container registry this hostname
	// is expected to be accessible from the host machine.
	PushEndpoint string `mapstructure:"pushendpoint" validate:"required"`

	// PullEndpoint is the endpoing used to pull by the runtime. For a local container registry this hostname
	// is expected to be accessible by the runtime. Can be the same as PushEndpoint if the registry has a routable
	// address.
	PullEndpoint string `mapstructure:"pullendpoint" validate:"required"`
}
