// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"
	"fmt"

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
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"
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
	GetSetupInterface() setup.Interface
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
	SetupInterface      setup.Interface
}

func (i *Impl) GetBicep() bicep.Interface {
	return i.Bicep
}

func (i *Impl) GetConnectionFactory() connections.Factory {
	return i.ConnectionFactory
}

func (i *Impl) GetConfigHolder() *ConfigHolder {
	return i.ConfigHolder
}

func (i *Impl) GetDeploy() deploy.Interface {
	return i.Deploy
}

func (i *Impl) GetLogstream() logstream.Interface {
	return i.Logstream
}

func (i *Impl) GetOutput() output.Interface {
	return i.Output
}

// GetPortforward fetches the portforward interface.
func (i *Impl) GetPortforward() portforward.Interface {
	return i.Portforward
}

// GetPrompter fetches the interface to bubble tea prompt
func (i *Impl) GetPrompter() prompt.Interface {
	return i.Prompter
}

// GetConfigFileInterface fetches the interface to interace with radius config file
func (i *Impl) GetConfigFileInterface() ConfigFileInterface {
	return i.ConfigFileInterface
}

// GetKubernetesInterface fetches the interface to get info related to the kubernetes cluster
func (i *Impl) GetKubernetesInterface() kubernetes.Interface {
	return i.KubernetesInterface
}

// GetHelmInterface fetches the interface for operations related to radius installation
func (i *Impl) GetHelmInterface() helm.Interface {
	return i.HelmInterface
}

// GetNamespaceInterface fetches the interface for operations related to radius installation
func (i *Impl) GetNamespaceInterface() namespace.Interface {
	return i.NamespaceInterface
}

func (i *Impl) GetSetupInterface() setup.Interface {
	return i.SetupInterface
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
			sc := trace.SpanContextFromContext(cmd.Context())
			return fmt.Errorf("%w \n traceId is %s", err, sc.TraceID())
		}

		return nil
	}
}
