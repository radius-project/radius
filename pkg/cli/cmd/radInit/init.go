// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	Azure int = iota
	AWS
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Installs rad with an env creation",
		Long:    "Installs rad with an env creation",
		Example: `rad env init`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	// Define your flags here
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder           *framework.ConfigHolder
	Output                 output.Interface
	Format                 string
	Workspace              *workspaces.Workspace
	ServicePrincipal       *azure.ServicePrincipal
	KubeContext            string
	EnvName                string
	NameSpace              string
	SubscriptionID         string
	CloudProvider          string
	ResourceGroupName      string
	CreateNewResourceGroup bool
	IsRadiusInstalled      bool
	Reinstall              bool
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return &cli.FriendlyError{Message: "workspace not mentioned"}
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return &cli.FriendlyError{Message: "output format not mentioned"}
	}
	r.Format = format
	kubeContext, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to read kube config"}
	}
	fmt.Print(kubeContext.Contexts)
	//TODO: check flags if interactive or not
	r.KubeContext, err = selectKubeContext(kubeContext.CurrentContext, kubeContext.Contexts, true)
	if err != nil {
		return &cli.FriendlyError{Message: "KubeContext not mentioned"}
	}
	//TODO: check flags if interactive or not
	r.EnvName, err = common.SelectEnvironmentName(cmd, "default", true)
	if err != nil {
		return &cli.FriendlyError{Message: "Environment not mentioned"}
	}

	r.NameSpace, err = common.SelectNamespace(cmd, "default", true)
	if err != nil {
		return &cli.FriendlyError{Message: "Namespace not mentioned"}
	}

	addingProvider := true
	for addingProvider {
		msg := "Add cloud providers for cloud resources [y/N]?"
		cloudProvider, err := selectCloudProvider(r.Output, msg)
		if err != nil {
			return &cli.FriendlyError{Message: "Error reading cloud provider"}
		}

		switch cloudProvider {
		case Azure:
			msg = "Enter Azure subscription Id"
			r.SubscriptionID, err = prompt.Text(msg, nil)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading subscription Id for azure provider"}
			}

			msg = "Do you want to create an azure resource group"
			yes, err := prompt.ConfirmWithDefault(msg, prompt.No)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading resource group for azure provider"}
			}
			if yes {
				r.ResourceGroupName, err = prompt.Text("Enter a resource group for azure:", nil)
				r.CreateNewResourceGroup = true
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading resource group for azure provider"}
				}
			} else {
				//TODO: list groups from ucp and prompt the user after the functionality is implemented.
				r.CreateNewResourceGroup = false
			}
			yes, err = prompt.ConfirmWithDefault("Create a new Azure service principal [Y/n]", prompt.Yes)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading service principal for azure provider"}
			}
			if yes {
				appId, err := prompt.Text("Enter an appId for the service principal", nil)
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading service principal for azure provider"}
				}

				pwd, err := prompt.Text("Enter the password of the app", nil)
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading service principal for azure provider"}
				}

				tenantId, err := prompt.Text("Enter tenantId of the app", nil)
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading service principal for azure provider"}
				}

				r.ServicePrincipal = &azure.ServicePrincipal{ClientID: appId, ClientSecret: pwd, TenantID: tenantId}
			}
			addingProvider, err = prompt.ConfirmWithDefault("Would you like to add another cloud provider [y/N]", prompt.No)
			if err != nil {
				return &cli.FriendlyError{Message: "Failed to read confirmation"}
			}
		case AWS:
			r.Output.LogInfo("AWS is not supported")
		}
		r.IsRadiusInstalled, err = helm.CheckRadiusInstall(r.KubeContext)
		if err != nil {
			return &cli.FriendlyError{Message: "Failed to check for radius installation"}
		}
		if r.IsRadiusInstalled {
			msg = "Radius control-plane already installed in context 'AKS' with version '0.12' Would you like to reinstall Radius control-plane with the latest version [Y/n]? Y"
			prompt.ConfirmWithDefault(msg, prompt.No)
		}
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	// TODO: Create ucp resource group if a new one is provided
	// Install radius control plane if radius is not installed or user prompts to reinstall
	if !r.IsRadiusInstalled || r.Reinstall {
		err := installRadius(ctx, r.Reinstall, r)
		if err != nil {
			return &cli.FriendlyError{Message: "Failed to install radius"}
		}
	} else {
		// azureProvider := azure.Provider{
		// 	SubscriptionID:   r.SubscriptionID,
		// 	ResourceGroup:    r.ResourceGroupName,
		// 	ServicePrincipal: r.ServicePrincipal,
		// }
		
	}

	// create cloud provider if needed
	// install radius
	return nil
}

func installRadius(ctx context.Context, reinstallRad bool, runner *Runner) error {
	azureProvider := azure.Provider{
		SubscriptionID:   runner.SubscriptionID,
		ResourceGroup:    runner.ResourceGroupName,
	}
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall:     reinstallRad,
			AzureProvider: &azureProvider,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	// Ignore existing radius installation because we already asked the user whether to re-install or not
	_, err := setup.Install(ctx, clusterOptions, runner.KubeContext)
	if err != nil {
		return err
	}
	return nil
}

func selectKubeContext(currentContext string, kubeContexts map[string]*api.Context, interactive bool) (string, error) {
	values := []string{}
	if interactive {
		for k, _ := range kubeContexts {
			values = append(values, k)
		}
		index, err := prompt.SelectWithDefault("Select kubeContext:", &currentContext, values)
		if err != nil {
			return "", err
		}

		return values[index], nil
	}
	return currentContext, nil
}

func selectCloudProvider(output output.Interface, selectionMessage string) (int, error) {
	yes := true
	values := []string{"Azure", "AWS", "[back]"}
	var index int
	for yes {
		yes, err := prompt.ConfirmWithDefault(selectionMessage, prompt.No)
		if err != nil {
			return -1, err
		}
		if yes {
			index, err = prompt.SelectWithDefault("", &values[0], values)
			if err != nil {
				return -1, err
			}
			if values[index] == "AWS" {
				output.LogInfo("AWS not supported")
				continue
			}
			if values[index] == "[back]" {
				continue
			}
			yes = !yes
		}
	}
	return index, nil
}

func selectResourceGroup(output output.Interface, selectionMessage string) (int, error) {
	yes := true
	values := []string{"Azure", "AWS", "[back]"}
	var index int
	for yes {
		yes, err := prompt.ConfirmWithDefault(selectionMessage, prompt.No)
		if err != nil {
			return -1, err
		}
		if yes {
			index, err = prompt.SelectWithDefault("", &values[0], values)
			if err != nil {
				return -1, err
			}
			if values[index] == "AWS" {
				output.LogInfo("AWS not supported")
				continue
			}
			if values[index] == "[back]" {
				continue
			}
			yes = !yes
		}
	}
	return index, nil
}
