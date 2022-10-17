// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// Factory interface handles resources for interfacing with corerp and configs
type Factory interface {
	GetConnectionFactory() connections.Factory
	GetConfigHolder() *ConfigHolder
	GetOutput() output.Interface
	GetPrompter() prompt.Interface
	GetConfigFileInterface() ConfigFileInterface
	GetKubernetesInterface() kubernetes.Interface
	GetHelmInterface() helm.Interface
	GetNamespaceInterface() namespace.Interface
	GetAppManagementClient() clients.ApplicationsManagementClient
}

type Impl struct {
	ConnectionFactory   connections.Factory
	ConfigHolder        *ConfigHolder
	Output              output.Interface
	Prompter            prompt.Interface
	ConfigFileInterface ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	HelmInterface       helm.Interface
	NamespaceInterface  namespace.Interface
	AppManagementClient clients.ApplicationsManagementClient
}

func (i *Impl) GetConnectionFactory() connections.Factory {
	return i.ConnectionFactory
}

func (i *Impl) GetConfigHolder() *ConfigHolder {
	return i.ConfigHolder
}

func (i *Impl) GetOutput() output.Interface {
	return i.Output
}

// Fetches the interface to prompt user for values
func (i *Impl) GetPrompter() prompt.Interface {
	return i.Prompter
}

// Fetches the interface to interace with radius config file
func (i *Impl) GetConfigFileInterface() ConfigFileInterface {
	return i.ConfigFileInterface
}

// Fetches the interface to get info related to the kubernetes cluster
func (i *Impl) GetKubernetesInterface() kubernetes.Interface {
	return i.KubernetesInterface
}

// Fetches the interface for operations related to radius installation
func (i *Impl) GetHelmInterface() helm.Interface {
	return i.HelmInterface
}

// Fetches the interface for operations related to radius installation
func (i *Impl) GetNamespaceInterface() namespace.Interface {
	return i.NamespaceInterface
}

func (i *Impl) GetAppManagementClient() clients.ApplicationsManagementClient {
	return i.AppManagementClient
}

type Runner interface {
	Validate(cmd *cobra.Command, args []string) error
	Run(ctx context.Context) error
}

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
