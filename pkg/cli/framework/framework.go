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

package framework

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/kubernetes/logstream"
	"github.com/project-radius/radius/pkg/cli/kubernetes/portforward"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// Factory interface handles resources for interfacing with corerp and configs
type Factory interface {
	GetBicep() bicep.Interface
	GetConnectionFactory() connections.Factory
	GetConfigHolder() *ConfigHolder
	GetDeploy() deploy.Interface
	GetLogstream() logstream.Interface
	GetOutput() output.Interface

	// GetPortforward fetches the portforward interface.
	GetPortforward() portforward.Interface
	GetPrompter() prompt.Interface
	GetConfigFileInterface() ConfigFileInterface
	GetKubernetesInterface() kubernetes.Interface
	GetHelmInterface() helm.Interface
	GetNamespaceInterface() namespace.Interface

	// GetAWSClient returns the AWS Client.
	GetAWSClient() aws.Client

	// GetAzureClient returns the Azure Client.
	GetAzureClient() azure.Client
}

type Impl struct {
	Bicep               bicep.Interface
	ConnectionFactory   connections.Factory
	ConfigHolder        *ConfigHolder
	Deploy              deploy.Interface
	Logstream           logstream.Interface
	Output              output.Interface
	Portforward         portforward.Interface
	Prompter            prompt.Interface
	ConfigFileInterface ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	HelmInterface       helm.Interface
	NamespaceInterface  namespace.Interface

	// AWSClient is the client for AWS.
	AWSClient aws.Client

	// AzureClient is the client for Azure.
	AzureClient azure.Client
}

// GetBicep() returns the Bicep interface stored in the Impl struct.
func (i *Impl) GetBicep() bicep.Interface {
	return i.Bicep
}

// GetConnectionFactory returns the ConnectionFactory stored in the Impl struct.
func (i *Impl) GetConnectionFactory() connections.Factory {
	return i.ConnectionFactory
}

// GetConfigHolder() returns a pointer to the ConfigHolder struct stored in the Impl struct.
func (i *Impl) GetConfigHolder() *ConfigHolder {
	return i.ConfigHolder
}

// GetDeploy() returns the Deploy interface stored in the Impl struct.
func (i *Impl) GetDeploy() deploy.Interface {
	return i.Deploy
}

// GetLogstream returns the logstream.Interface stored in the Impl struct.
func (i *Impl) GetLogstream() logstream.Interface {
	return i.Logstream
}

// GetOutput() returns the Output interface stored in the Impl struct.
func (i *Impl) GetOutput() output.Interface {
	return i.Output
}

// GetPortforward fetches the portforward interface.
//

// GetPortforward() returns the portforward.Interface stored in the Impl struct.
func (i *Impl) GetPortforward() portforward.Interface {
	return i.Portforward
}

// GetPrompter fetches the interface to bubble tea prompt
//

// GetPrompter() returns the Prompter field of the Impl struct, which is of type prompt.Interface.
func (i *Impl) GetPrompter() prompt.Interface {
	return i.Prompter
}

// GetConfigFileInterface fetches the interface to interace with radius config file
//

// GetConfigFileInterface returns the ConfigFileInterface stored in the Impl struct.
func (i *Impl) GetConfigFileInterface() ConfigFileInterface {
	return i.ConfigFileInterface
}

// GetKubernetesInterface fetches the interface to get info related to the kubernetes cluster
//

// GetKubernetesInterface returns the KubernetesInterface stored in the Impl struct.
func (i *Impl) GetKubernetesInterface() kubernetes.Interface {
	return i.KubernetesInterface
}

// GetHelmInterface fetches the interface for operations related to radius installation
//

// GetHelmInterface returns the helm.Interface stored in the Impl struct.
func (i *Impl) GetHelmInterface() helm.Interface {
	return i.HelmInterface
}

// GetNamespaceInterface fetches the interface for operations related to radius installation
//

// GetNamespaceInterface returns the namespace interface stored in the Impl struct.
func (i *Impl) GetNamespaceInterface() namespace.Interface {
	return i.NamespaceInterface
}

// GetAWSClient returns the AWS Client.
//

// GetAWSClient returns the AWSClient stored in the Impl struct.
func (i *Impl) GetAWSClient() aws.Client {
	return i.AWSClient
}

// GetAzureClient returns the Azure Client.
//

// GetAzureClient returns an azure.Client from the Impl struct.
func (i *Impl) GetAzureClient() azure.Client {
	return i.AzureClient
}

type Runner interface {
	Validate(cmd *cobra.Command, args []string) error
	Run(ctx context.Context) error
}

// RunCommand is a function that takes in a Runner interface and returns a function that takes in a cobra command and a
// slice of strings, and returns an error if one occurs during validation or running.
func RunCommand(runner Runner) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := runner.Validate(cmd, args)
		if err != nil {
			return err
		}

		err = runner.Run(cmd.Context())
		if err != nil {
			return err
		}

		return nil
	}
}
