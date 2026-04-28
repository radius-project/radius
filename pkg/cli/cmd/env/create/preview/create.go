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

package preview

import (
	"context"

	"github.com/spf13/cobra"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

// NewCommand creates an instance of the command and runner for the `rad env create` command.

// NewCommand creates a new Cobra command and a Runner object to handle the command's logic, and adds flags to the command
// for environment name, workspace, resource group, and cloud provider configuration.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [envName]",
		Short: "Create a new Radius Environment",
		Long: `Create a new Radius Environment
Radius Environments are prepared "landing zones" for Radius Applications.
Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.`,
		Args: cobra.ExactArgs(1),
		Example: `
## Create environment
rad env create myenv

## Create environment with Azure cloud provider
rad env create myenv --azure-subscription-id **** --azure-resource-group myrg

## Create environment with AWS cloud provider
rad env create myenv --aws-region us-west-2 --aws-account-id *****

## Create environment with Kubernetes cloud provider
rad env create myenv --kubernetes-namespace mynamespace

## Create environment with Kubernetes cloud provider using deprecated --namespace alias
rad env create myenv --namespace mynamespace
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddAzureSubscriptionFlag(cmd)
	commonflags.AddAzureResourceGroupFlag(cmd)
	cmd.MarkFlagsRequiredTogether(commonflags.AzureSubscriptionIdFlag, commonflags.AzureResourceGroupFlag)
	commonflags.AddAWSRegionFlag(cmd)
	commonflags.AddAWSAccountFlag(cmd)
	cmd.MarkFlagsRequiredTogether(commonflags.AWSRegionFlag, commonflags.AWSAccountIdFlag)
	commonflags.AddKubernetesNamespaceFlag(cmd)
	commonflags.AddNamespaceFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive(commonflags.KubernetesNamespaceFlag, "namespace")
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env create` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	Workspace               *workspaces.Workspace
	EnvironmentName         string
	ResourceGroupName       string
	RadiusCoreClientFactory *corerpv20250801.ClientFactory
	// DefaultScopeClientFactory is a client factory scoped to the default resource group.
	// The default recipe pack is always created in this scope. If nil, it will be
	// initialized automatically.
	DefaultScopeClientFactory *corerpv20250801.ClientFactory
	ConfigFileInterface       framework.ConfigFileInterface
	ConnectionFactory         connections.Factory

	providers *corerpv20250801.Providers
}

// NewRunner creates a new instance of the `rad env create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		ConnectionFactory:   factory.GetConnectionFactory(),
	}
}

// Validate runs validation for the `rad env create` command.
// Validate checks if the workspace, environment name, scope, resource group, and cloud provider flags
// are valid and returns an error if any of them are invalid or missing.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {

	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	// Parse the resource group name so we can use it later. DO NOT use the
	// --group argument, because we want to find the right group when the user
	// didn't pass it.
	scopeId, err := resources.Parse(r.Workspace.Scope)
	if err != nil {
		return err
	}
	r.ResourceGroupName = scopeId.FindScope(resources_radius.ScopeResourceGroups)

	_, err = client.GetResourceGroup(cmd.Context(), "local", r.ResourceGroupName)
	if clients.Is404Error(err) {
		return clierrors.Message("Resource group %q could not be found.", r.ResourceGroupName)
	} else if err != nil {
		return err
	}

	// Initialize providers
	r.providers = &corerpv20250801.Providers{
		Azure:      &corerpv20250801.ProvidersAzure{},
		Aws:        &corerpv20250801.ProvidersAws{},
		Kubernetes: &corerpv20250801.ProvidersKubernetes{},
	}

	// Validate Azure scope components
	if cmd.Flags().Changed(commonflags.AzureSubscriptionIdFlag) || cmd.Flags().Changed(commonflags.AzureResourceGroupFlag) {
		azureSubId, err := cli.RequireAzureSubscriptionId(cmd)
		if err != nil {
			return err
		}

		azureRgId, err := cmd.Flags().GetString(commonflags.AzureResourceGroupFlag)
		if err != nil {
			return err
		}

		r.providers.Azure.SubscriptionID = new(azureSubId)
		r.providers.Azure.ResourceGroupName = new(azureRgId)
	}

	// Validate AWS scope components
	if cmd.Flags().Changed(commonflags.AWSRegionFlag) || cmd.Flags().Changed(commonflags.AWSAccountIdFlag) {
		awsRegion, err := cmd.Flags().GetString(commonflags.AWSRegionFlag)
		if err != nil {
			return err
		}

		awsAccountId, err := cmd.Flags().GetString(commonflags.AWSAccountIdFlag)
		if err != nil {
			return err
		}

		r.providers.Aws.Region = new(awsRegion)
		r.providers.Aws.AccountID = new(awsAccountId)
	}

	// Validate Kubernetes scope components. Accept either --kubernetes-namespace
	// or the backward-compatible --namespace alias (the two are mutually exclusive).
	if cmd.Flags().Changed(commonflags.KubernetesNamespaceFlag) {
		k8sNamespace, err := cmd.Flags().GetString(commonflags.KubernetesNamespaceFlag)
		if err != nil {
			return err
		}

		r.providers.Kubernetes.Namespace = new(k8sNamespace)
	} else if cmd.Flags().Changed("namespace") {
		k8sNamespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return err
		}

		r.providers.Kubernetes.Namespace = new(k8sNamespace)
	}

	return nil
}

// Run runs the `rad env create --preview` command.
//
// Run creates a new Radius.Core environment with the default recipe pack
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	// Ensure the default resource group exists before creating recipe pack in it.
	mgmtClient, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	if err := recipepack.EnsureDefaultResourceGroup(ctx, mgmtClient.CreateOrUpdateResourceGroup); err != nil {
		return err
	}

	// Create the default recipe pack in the default resource group.
	// The default pack lives in the default scope regardless of the current workspace scope.
	if r.DefaultScopeClientFactory == nil {
		defaultClientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, recipepack.DefaultResourceGroupScope)
		if err != nil {
			return err
		}
		r.DefaultScopeClientFactory = defaultClientFactory
	}

	recipePackClient := r.DefaultScopeClientFactory.NewRecipePacksClient()
	_, err = recipepack.GetOrCreateDefaultRecipePack(ctx, recipePackClient)
	if err != nil {
		return err
	}

	properties := &corerpv20250801.EnvironmentProperties{
		RecipePacks: []*string{to.Ptr(recipepack.DefaultRecipePackID())},
	}

	// Set providers if any were configured.
	if r.providers != nil {
		hasAzure := r.providers.Azure != nil && (r.providers.Azure.SubscriptionID != nil || r.providers.Azure.ResourceGroupName != nil)
		hasAws := r.providers.Aws != nil && (r.providers.Aws.AccountID != nil || r.providers.Aws.Region != nil)
		hasK8s := r.providers.Kubernetes != nil && r.providers.Kubernetes.Namespace != nil
		if hasAzure || hasAws || hasK8s {
			properties.Providers = &corerpv20250801.Providers{}
			if hasAzure {
				properties.Providers.Azure = r.providers.Azure
			}
			if hasAws {
				properties.Providers.Aws = r.providers.Aws
			}
			if hasK8s {
				properties.Providers.Kubernetes = r.providers.Kubernetes
			}
		}
	}

	resource := &corerpv20250801.EnvironmentResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: properties,
	}

	envClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()
	_, err = envClient.CreateOrUpdate(ctx, r.EnvironmentName, *resource, nil)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Radius.Core/environments/%s created", r.EnvironmentName)
	return nil
}
