// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
)

const (
	KindAzureCloud                   = "azure"
	KindKubernetes                   = "kubernetes"
	EnvironmentKeyDefaultApplication = "defaultapplication"
)

type Environment interface {
	GetName() string
	GetKind() string
	GetDefaultApplication() string
	GetKubeContext() string
	GetId() string

	// GetStatusLink provides an optional URL for display of the environment.
	GetStatusLink() string

	// GetContainerRegistry provides an optional container registry override. The registry is used
	// by the 'rad app ...' family of commands for development purposes.
	GetContainerRegistry() *Registry

	GetProviders() *Providers
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

type Providers struct {
	AzureProvider *azure.Provider `mapstructure:"azure,omitempty"`
	AWSProvider   *aws.Provider   `mapstructure:"aws,omitempty"`
}
