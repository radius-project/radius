// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/cmd"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	azureCloudProvider = "Azure"
	awsCloudProvider   = "AWS"
	backNavigator      = "[back]"

	confirmCloudProviderPrompt    = "Add cloud providers for cloud resources?"
	confirmReinstallRadiusPrompt  = "Would you like to reinstall Radius control plane and configure cloud providers?"
	confirmSetupApplicationPrompt = "Setup application in the current directory?"
	enterApplicationName          = "Choose an application name"
	selectKubeContextPrompt       = "Select the kubeconfig context to install Radius into"
	selectCloudProviderPrompt     = "Select your cloud provider"
	kubernetesKind                = "kubernetes"
)

// NewCommand creates an instance of the command and runner for the `rad init` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "init",
		Short:   "Initialize Radius",
		Long:    "Interactively initialize the Radius control-plane, create an environment, and configure a workspace",
		Example: `rad init`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	// Define your flags here
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().Bool("dev", false, "Setup Radius for development")
	cmd.Flags().Bool("skip-dev-recipes", false, "Use this flag to not use radius built in recipes")
	return cmd, runner
}

// Runner is the runner implementation for the `rad init` command.
type Runner struct {
	ConfigFileInterface framework.ConfigFileInterface
	ConfigHolder        *framework.ConfigHolder
	ConnectionFactory   connections.Factory
	HelmInterface       helm.Interface
	KubernetesInterface kubernetes.Interface
	Output              output.Interface
	Prompter            prompt.Interface
	SetupInterface      setup.Interface

	Format                  string
	AzureCloudProvider      *azure.Provider
	ExistingEnvironment     bool
	EnvName                 string
	KubeContext             string
	Namespace               string
	RadiusInstalled         bool
	Reinstall               bool
	ScaffoldApplication     bool
	ScaffoldApplicationName string
	ServicePrincipal        *azure.ServicePrincipal
	SkipDevRecipes          bool
	Workspace               *workspaces.Workspace
	Dev                     bool
}

// NewRunner creates a new instance of the `rad init` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		Prompter:            factory.GetPrompter(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		HelmInterface:       factory.GetHelmInterface(),
		SetupInterface:      factory.GetSetupInterface(),
	}
}

// Validate runs validation for the `rad init` command.
//
// Validates the user prompts, values provided and builds the picture for the backend to execute
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return &cli.FriendlyError{Message: "Output format not specified"}
	}
	r.Format = format

	r.Dev, err = cmd.Flags().GetBool("dev")
	if err != nil {
		return err
	}

	kubeContextList, err := r.KubernetesInterface.GetKubeContext()
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to read kube config"}
	}

	// In dev mode we will just take the default kubecontext
	r.KubeContext, err = selectKubeContext(kubeContextList.CurrentContext, kubeContextList.Contexts, !r.Dev, r.Prompter)
	if err != nil {
		return &cli.FriendlyError{Message: "KubeContext not specified"}
	}

	r.SkipDevRecipes, err = cmd.Flags().GetBool("skip-dev-recipes")
	if err != nil {
		return err
	}

	r.RadiusInstalled, err = r.HelmInterface.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return &cli.FriendlyError{Message: "Unable to verify radius installation on cluster"}
	}

	if r.RadiusInstalled && !r.Dev {
		output.LogInfo(fmt.Sprintf("Radius control plane is already installed to context '%s'...", r.KubeContext))
		y, err := prompt.YesOrNoPrompt(confirmReinstallRadiusPrompt, "no", r.Prompter)
		if err != nil {
			return &cli.FriendlyError{Message: "Unable to read reinstall prompt"}
		}
		if y {
			r.Reinstall = true
		}
	}

	// Set up a connection so we can list environments
	r.Workspace = &workspaces.Workspace{
		Connection: map[string]any{
			"context": r.KubeContext,
			"kind":    kubernetesKind,
		},

		// We can't know the scope yet. Setting it up likes this ensures that any code
		// that needs a resource group will fail. After we know the env name we will
		// update this value.
		Scope: "/planes/radius/local",
	}

	environments := []corerp.EnvironmentResource{}
	if r.RadiusInstalled {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
		if err != nil {
			return err
		}

		environments, err = client.ListEnvironmentsAll(cmd.Context())
		if err != nil {
			return err
		}
	}

	// If there are any existing environments and we're not reinstalling, ask to use
	// one of those first.
	//
	// "reinstall" repreresents the the user-intent to reconfigure cloud providers,
	// we also need to force re-creation of the envionment to do that, so we don't want
	// to reuse an existing one.
	if len(environments) > 0 && !r.Reinstall {

		// In dev mode, we take the default without asking if it's an option.
		//
		// The best way to accomplish that is to run SelectedExistingEnvironment in non-interactive mode
		// first, and then try again interactively if we get no results.
		if r.Dev {
			r.EnvName, err = common.SelectExistingEnvironment(cmd, "default", false, r.Prompter, environments)
			if err != nil {
				return err
			}
		}

		if r.EnvName == "" {
			r.EnvName, err = common.SelectExistingEnvironment(cmd, "default", true, r.Prompter, environments)
			if err != nil {
				return err
			}
		}

		// User choose an existing environment, grab any settings we need from it.
		if r.EnvName != "" {
			r.ExistingEnvironment = true

			// Grab any provider info we found on the environment resource so we can store it locally.
			for _, env := range environments {
				if strings.EqualFold(r.EnvName, *env.Name) {
					if env.Properties != nil &&
						env.Properties.Providers != nil &&
						env.Properties.Providers.Azure != nil &&
						env.Properties.Providers.Azure.Scope != nil {
						scope, err := resources.ParseScope(*env.Properties.Providers.Azure.Scope)
						if err != nil {
							return err
						}

						r.AzureCloudProvider = &azure.Provider{
							SubscriptionID: scope.FindScope(resources.SubscriptionsSegment),
							ResourceGroup:  scope.FindScope(resources.ResourceGroupsSegment),
						}
					}
					break
				}
			}
		}
	}

	// If we're going to create an environment, then prompt for the name now.
	if !r.ExistingEnvironment {
		// In dev mode don't ask for a name, just use 'default'
		if r.Dev {
			r.EnvName = "default"
		} else {
			r.EnvName, err = common.SelectEnvironmentName(cmd, "default", true, r.Prompter)
			if err != nil {
				return &cli.FriendlyError{Message: "Failed to read env name"}
			}
		}

		// In dev mode we don't want to ask about namespaces or cloud providers
		if r.Dev {
			r.Namespace = "default"
		} else {
			r.Namespace, err = common.SelectNamespace(cmd, "default", true, r.Prompter)
			if err != nil {
				return &cli.FriendlyError{Message: "Namespace not specified"}
			}

			// Configuring Cloud Provider
			addingCloudProvider, err := prompt.YesOrNoPrompt(confirmCloudProviderPrompt, "no", r.Prompter)
			if err != nil {
				return &cli.FriendlyError{Message: "Error reading cloud provider"}
			}
			for addingCloudProvider {
				cloudProvider, err := selectCloudProvider(r.Prompter)
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading cloud provider"}
				}
				switch cloudProvider {
				case azureCloudProvider:
					r.AzureCloudProvider, err = r.SetupInterface.ParseAzureProviderArgs(cmd, true, r.Prompter)
					if err != nil {
						return err
					}
				case awsCloudProvider:
					r.Output.LogInfo("AWS is not supported")
				case backNavigator:
					break
				default:
					return &cli.FriendlyError{Message: "Unsupported Cloud Provider"}
				}
				addingCloudProvider, err = prompt.YesOrNoPrompt(confirmCloudProviderPrompt, "no", r.Prompter)
				if err != nil {
					return &cli.FriendlyError{Message: "Error reading cloud provider"}
				}
			}
		}
	}

	// Update the workspace with the information we captured about the environment.
	r.Workspace.Name = r.EnvName
	r.Workspace.Environment = fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", r.EnvName, r.EnvName)
	r.Workspace.Scope = fmt.Sprintf("/planes/radius/local/resourceGroups/%s", r.EnvName)

	r.ScaffoldApplication, err = prompt.YesOrNoPrompt(confirmSetupApplicationPrompt, "Yes", r.Prompter)
	if err != nil {
		return err
	}

	if r.ScaffoldApplication {
		r.ScaffoldApplicationName, err = chooseApplicationName(r.Prompter)
		if err != nil {
			return err
		}
	}

	return nil
}

// Run runs the `rad init` command.
//
// Creates radius resources, azure resources if required based on the user input, command flags
func (r *Runner) Run(ctx context.Context) error {
	config := r.ConfigFileInterface.ConfigFromContext(ctx)
	//TODO: Initialize cloud providers separately once providers commands are in
	// If the user prompts for re-install, re-install and init providers
	// If the user says no, then use the provider create/update operations to update the provider config.
	// issue: https://github.com/project-radius/radius/issues/3440
	if r.Reinstall || !r.RadiusInstalled {
		// Install radius control plane
		err := installRadius(ctx, r)
		if err != nil {
			return &cli.FriendlyError{Message: "Failed to install radius"}
		}
	}

	if r.ExistingEnvironment {
		r.Output.LogInfo("Using existing environment %s...", r.EnvName)
	} else {
		r.Output.LogInfo("Creating environment %s...", r.EnvName)
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}

		//ignore the id of the resource group created
		isGroupCreated, err := client.CreateUCPGroup(ctx, "radius", "local", r.EnvName, v20220901privatepreview.ResourceGroupResource{
			Location: to.Ptr(v1.LocationGlobal),
		})
		if err != nil || !isGroupCreated {
			return &cli.FriendlyError{Message: "Failed to create ucp resource group"}
		}

		// TODO: we TEMPORARILY create a resource group in the deployments plane because the deployments RP requires it.
		// We'll remove this in the future.
		_, err = client.CreateUCPGroup(ctx, "deployments", "local", r.EnvName, v20220901privatepreview.ResourceGroupResource{
			Location: to.Ptr(v1.LocationGlobal),
		})
		if err != nil {
			return err
		}

		// create the providers scope from the AzureCloudProvider properties for creating the environment
		var providers corerp.Providers
		if r.AzureCloudProvider != nil {
			providers = cmd.CreateEnvAzureProvider(r.AzureCloudProvider.SubscriptionID, r.AzureCloudProvider.ResourceGroup)
		}

		isEnvCreated, err := client.CreateEnvironment(ctx, r.EnvName, v1.LocationGlobal, r.Namespace, "kubernetes", "", map[string]*corerp.EnvironmentRecipeProperties{}, &providers, !r.SkipDevRecipes)
		if err != nil || !isEnvCreated {
			return &cli.FriendlyError{Message: "Failed to create radius environment"}
		}
	}

	err := r.ConfigFileInterface.EditWorkspaces(ctx, config, r.Workspace, r.AzureCloudProvider)
	if err != nil {
		return err
	}

	if r.ScaffoldApplication {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}

		// Initialize the application resource if it's not found. This supports the scenario where the application
		// resource is not defined in bicep.
		err = client.CreateApplicationIfNotFound(ctx, r.ScaffoldApplicationName, corerp.ApplicationResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &corerp.ApplicationProperties{
				Environment: &r.Workspace.Environment,
			},
		})
		if err != nil {
			return err
		}

		// Scaffold application files in the current directory
		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		err = setup.ScaffoldApplication(r.Output, wd, r.ScaffoldApplicationName)
		if err != nil {
			return err
		}
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
	_, err := r.HelmInterface.InstallRadius(ctx, clusterOptions, r.KubeContext)
	if err != nil {
		return err
	}

	return nil
}

func selectKubeContext(currentContext string, kubeContexts map[string]*api.Context, interactive bool, prompter prompt.Interface) (string, error) {
	values := []string{}
	if interactive {
		// Ensure current context is at the top as the default
		values = append(values, currentContext)
		for k := range kubeContexts {
			if k != currentContext {
				values = append(values, k)
			}
		}
		kubeContext, err := prompter.GetListInput(values, selectKubeContextPrompt)
		if err != nil {
			return "", err
		}
		return kubeContext, nil
	}

	return currentContext, nil
}

// Selects the cloud provider, returns -1 if back and -2 if not supported
func selectCloudProvider(prompter prompt.Interface) (string, error) {
	values := []string{azureCloudProvider, awsCloudProvider, backNavigator}
	return prompter.GetListInput(values, selectCloudProviderPrompt)
}

func chooseApplicationName(prompter prompt.Interface) (string, error) {
	// We might have to prompt for an application name if the current directory is not a valid application name.
	// These cases should be rare but just in case...
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	directory := filepath.Base(wd)
	valid, _, err := prompt.ResourceName(directory)
	if err != nil {
		return "", err
	}

	if valid {
		return directory, nil
	}

	appName, err := prompter.GetTextInput(enterApplicationName, "enter app name...")
	if err != nil {
		return "", nil
	}
	isValid, errMsg, _ := prompt.ResourceName(appName)
	if !isValid {
		return "", &cli.FriendlyError{Message: errMsg}
	}
	return appName, nil
}
