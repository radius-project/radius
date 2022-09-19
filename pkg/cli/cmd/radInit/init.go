// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
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
	ConnectionFactory      connections.Factory
	KubeContext            string
	EnvName                string
	NameSpace              string
	AzureCloudProvider     *azure.Provider
	CreateNewResourceGroup bool
	RadiusInstalled        bool
	Reinstall              bool
	Prompter               prompt.Interface
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Prompter:          &prompt.Impl{},
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
	//TODO: check flags if interactive or not
	r.KubeContext, err = selectKubeContext(kubeContext.CurrentContext, kubeContext.Contexts, true, r.Prompter)
	if err != nil {
		return &cli.FriendlyError{Message: "KubeContext not mentioned"}
	}
	//TODO: check flags if interactive or not
	r.EnvName, err = common.SelectEnvironmentName(cmd, "default", true, r.Prompter)
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to read env name"}
	}

	r.NameSpace, err = common.SelectNamespace(cmd, "default", true, r.Prompter)
	if err != nil {
		return &cli.FriendlyError{Message: "Namespace not mentioned"}
	}

	addingProvider := "y"
	for strings.ToLower(addingProvider) == "y" {
		var cloudProvider int
		addingCloudProvider := true
		for addingCloudProvider {
			cloudProviderPrompter, err := prompt.YesOrNoPrompter("Add cloud providers for cloud resources [y/N]?", "N", r.Prompter)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading cloud provider"}
			}
			if strings.ToLower(cloudProviderPrompter) == "n" {
				cloudProvider = -1
				break
			}
			cloudProvider, err := selectCloudProvider(r.Output, r.Prompter)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading cloud provider"}
			}
			if cloudProvider != -1 {
				addingCloudProvider = false
			}
		}
		if cloudProvider == -1 {
			break
		}
		switch cloudProvider {
		case Azure:
			//TODO: check for interactive flag
			r.AzureCloudProvider, err = setup.ParseAzureProviderArgs(cmd, true, r.Prompter)
			if err != nil {
				return err
			}
		case AWS:
			r.Output.LogInfo("AWS is not supported")
		}
		addingProvider, err = r.Prompter.RunPrompt(prompt.TextPromptWithDefault(
			"Would you like to add another cloud provider [y/N]",
			"N",
			nil,
		))
		if err != nil {
			return &cli.FriendlyError{Message: "Failed to read confirmation"}
		}
	}
	r.RadiusInstalled, err = helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return &cli.FriendlyError{Message: "Unable to verify radius installation on cluster"}
	}
	//TODO: prompt for re-install of radius once the provider commands are in
	// If the user prompts for re-install, then go ahead
	// If the user says no, then use the provider create/update operations to update the provider config.

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	// Install radius control plane
	// TODO: Add check for user prompts (whether user has prompted to re-install or not), 
	// if not then use provider operations to update provider and avoid re-installing radius control plane
	err := installRadius(ctx, r)
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to install radius"}
	}
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	isGroupCreated, err := client.CreateUCPGroup(ctx, "radius", "local", r.EnvName, v20220315privatepreview.ResourceGroupResource{})
	if err != nil || !isGroupCreated {
		return &cli.FriendlyError{Message: "Failed to create ucp resource group"}
	}

	isEnvCreated, err := client.CreateEnvironment(ctx, r.EnvName, "global", r.NameSpace, "Kubernetes", "")

	if err != nil || !isEnvCreated {
		return &cli.FriendlyError{Message: "Failed to create radius environment group"}
	}

	// Reload config so we can see the updates
	config, err := cli.LoadConfig(r.ConfigHolder.ConfigFilePath)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		ws := section.Items[strings.ToLower(r.Workspace.Name)]
		ws.Environment = r.EnvName
		section.Items[strings.ToLower(r.Workspace.Name)] = ws
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func installRadius(ctx context.Context, r *Runner) error {
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall:     r.Reinstall,
			AzureProvider: r.AzureCloudProvider,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	// Ignore existing radius installation because we already asked the user whether to re-install or not
	_, err := setup.Install(ctx, clusterOptions, r.KubeContext)
	if err != nil {
		return err
	}

	return nil
}

func selectKubeContext(currentContext string, kubeContexts map[string]*api.Context, interactive bool, prompter prompt.Interface) (string, error) {
	values := []string{}
	if interactive {
		confirmDefaultContext, err := prompt.YesOrNoPrompter(
			fmt.Sprintf("Confirm Default context: %s, %s", currentContext, "[Y/n]"),
			"Y",
			prompter,
		)
		if err != nil {
			return "", err
		}
		if strings.ToLower(confirmDefaultContext) == "y" {
			return currentContext, nil
		}
		for k := range kubeContexts {
			values = append(values, k)
		}
		index, _, err := prompter.RunSelect(prompt.SelectionPrompter(
			"Select the context for radius installation",
			values,
		))
		if err != nil {
			return "", err
		}

		return values[index], nil
	}
	return currentContext, nil
}

// Selects the cloud provider, returns -1 if back and -2 if not supported
func selectCloudProvider(output output.Interface, prompter prompt.Interface) (int, error) {
	values := []string{"Azure", "AWS", "[back]"}
	cloudProviderSelector := promptui.Select{
		Label: "Select your cloud provider",
		Items: values,
	}
	index, _, err := prompter.RunSelect(cloudProviderSelector)
	if err != nil {
		return -1, err
	}
	if values[index] == "AWS" {
		output.LogInfo("AWS not supported")
		return -2, nil
	}
	if values[index] == "[back]" {
		return -1, nil
	}
	return index, nil
}
