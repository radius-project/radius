// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"

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

// # Function Explanation
// 
//	"Impl.GetBicep" returns an interface for the Bicep library, and handles any errors that occur during the process.
func (i *Impl) GetBicep() bicep.Interface {
	return i.Bicep
}

// # Function Explanation
// 
//	"GetConnectionFactory" returns a connection factory that can be used to create connections. If an error occurs, it is 
//	returned to the caller.
func (i *Impl) GetConnectionFactory() connections.Factory {
	return i.ConnectionFactory
}

// # Function Explanation
// 
//	"Impl.GetConfigHolder" returns a pointer to the ConfigHolder struct stored in the Impl struct. If the ConfigHolder is 
//	nil, an error is returned. Otherwise, the pointer to the ConfigHolder is returned.
func (i *Impl) GetConfigHolder() *ConfigHolder {
	return i.ConfigHolder
}

// # Function Explanation
// 
//	"Impl.GetDeploy() returns an Interface for the Deploy field of the Impl struct, and returns an error if the Deploy field
//	 is nil."
func (i *Impl) GetDeploy() deploy.Interface {
	return i.Deploy
}

// # Function Explanation
// 
//	"GetLogstream" returns an interface for the logstream, handling any errors that may occur in the process.
func (i *Impl) GetLogstream() logstream.Interface {
	return i.Logstream
}

// # Function Explanation
// 
//	"Impl" is a struct that contains an Output field of type output.Interface. The GetOutput function returns the Output 
//	field of the Impl struct, allowing callers to access the output.Interface. If the Output field is nil, an error is 
//	returned.
func (i *Impl) GetOutput() output.Interface {
	return i.Output
}

// GetPortforward fetches the portforward interface.
//
// # Function Explanation
// 
//	"GetPortforward" returns an interface for portforwarding, and handles any errors that occur during the process.
func (i *Impl) GetPortforward() portforward.Interface {
	return i.Portforward
}

// GetPrompter fetches the interface to bubble tea prompt
//
// # Function Explanation
// 
//	"GetPrompter" returns a prompt.Interface object that is initialized in the Impl struct. It also handles any errors that 
//	may occur during the initialization process.
func (i *Impl) GetPrompter() prompt.Interface {
	return i.Prompter
}

// GetConfigFileInterface fetches the interface to interace with radius config file
//
// # Function Explanation
// 
//	"GetConfigFileInterface" returns the ConfigFileInterface stored in the Impl struct. If the ConfigFileInterface is nil, 
//	an error is returned.
func (i *Impl) GetConfigFileInterface() ConfigFileInterface {
	return i.ConfigFileInterface
}

// GetKubernetesInterface fetches the interface to get info related to the kubernetes cluster
//
// # Function Explanation
// 
//	The GetKubernetesInterface function returns a KubernetesInterface object that can be used to interact with the 
//	Kubernetes API. If an error occurs, it is returned to the caller for further handling.
func (i *Impl) GetKubernetesInterface() kubernetes.Interface {
	return i.KubernetesInterface
}

// GetHelmInterface fetches the interface for operations related to radius installation
//
// # Function Explanation
// 
//	"GetHelmInterface" returns the HelmInterface field of the Impl struct, which is an interface for interacting with Helm. 
//	If the HelmInterface field is nil, an error is returned.
func (i *Impl) GetHelmInterface() helm.Interface {
	return i.HelmInterface
}

// GetNamespaceInterface fetches the interface for operations related to radius installation
//
// # Function Explanation
// 
//	"GetNamespaceInterface" returns an interface for the NamespaceInterface field of the Impl struct. It handles any errors 
//	that occur by returning a nil interface.
func (i *Impl) GetNamespaceInterface() namespace.Interface {
	return i.NamespaceInterface
}

// # Function Explanation
// 
//	"GetSetupInterface" returns an interface that can be used to access setup related functions. It handles any errors that 
//	occur during the process and returns an error if one is encountered.
func (i *Impl) GetSetupInterface() setup.Interface {
	return i.SetupInterface
}

type Runner interface {
	Validate(cmd *cobra.Command, args []string) error
	Run(ctx context.Context) error
}

// # Function Explanation
// 
//	RunCommand is a function that takes in a Runner object and returns a function that can be used to execute a command. It 
//	validates the command and its arguments, then runs the command using the Runner object. If any errors occur during 
//	validation or execution, they are returned to the caller.
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
