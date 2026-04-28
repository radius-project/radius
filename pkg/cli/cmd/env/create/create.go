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

package create

import (
	"context"
	"fmt"

	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/spf13/cobra"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/env/namespace"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

const (
	azureScopeTemplate = "/subscriptions/%s/resourceGroups/%s"
	awsScopeTemplate   = "/planes/aws/aws/accounts/%s/regions/%s"
)

// NewCommand creates an instance of the command and runner for the `rad env create` command.
//

// NewCommand creates a new Cobra command and a Runner object to handle the command's logic, and adds flags to the command
// for environment name, workspace, resource group, namespace, and cloud provider configuration.
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
## Create environment with default namespace
rad env create myenv

## Create environment with a specific namespace
rad env create myenv --namespace mynamespace

## Create environment with Azure cloud provider
rad env create myenv --azure-subscription-id **** --azure-resource-group myrg

## Create environment with AWS cloud provider
rad env create myenv --aws-region us-west-2 --aws-account-id *****
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddNamespaceFlag(cmd)
	commonflags.AddAzureSubscriptionFlag(cmd)
	commonflags.AddAzureResourceGroupFlag(cmd)
	cmd.MarkFlagsRequiredTogether(commonflags.AzureSubscriptionIdFlag, commonflags.AzureResourceGroupFlag)
	commonflags.AddAWSRegionFlag(cmd)
	commonflags.AddAWSAccountFlag(cmd)
	cmd.MarkFlagsRequiredTogether(commonflags.AWSRegionFlag, commonflags.AWSAccountIdFlag)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env create` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Workspace           *workspaces.Workspace
	EnvironmentName     string
	ResourceGroupName   string
	Namespace           string
	ConnectionFactory   connections.Factory
	ConfigFileInterface framework.ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	NamespaceInterface  namespace.Interface

	providers *corerp.Providers
}

// NewRunner creates a new instance of the `rad env create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		NamespaceInterface:  factory.GetNamespaceInterface(),
	}
}

// Validate runs validation for the `rad env create` command.
//

// Validate checks if the workspace, environment name, scope, namespace, resource group name, namespace
// interface, and cloud provider flags are valid and returns an error if any of them are not.
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

	r.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if r.Namespace == "" {
		r.Namespace = r.EnvironmentName
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
	r.providers = &corerp.Providers{
		Azure: &corerp.ProvidersAzure{},
		Aws:   &corerp.ProvidersAws{},
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

		r.providers.Azure.Scope = new(fmt.Sprintf(azureScopeTemplate, azureSubId, azureRgId))
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

		r.providers.Aws.Scope = new(fmt.Sprintf(awsScopeTemplate, awsAccountId, awsRegion))
	}

	return nil
}

// Run runs the `rad env create` command.
//

// Run creates an environment in the specified resource group using the provided environment name, namespace,
// and cloud provider configuration, and returns an error if unsuccessful.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	properties := &corerp.EnvironmentProperties{
		Compute: &corerp.KubernetesCompute{
			Namespace: new(r.Namespace),
		},
	}

	// Set providers if any were configured.
	if r.providers != nil {
		hasAzure := r.providers.Azure != nil && r.providers.Azure.Scope != nil
		hasAws := r.providers.Aws != nil && r.providers.Aws.Scope != nil
		if hasAzure || hasAws {
			properties.Providers = &corerp.Providers{}
			if hasAzure {
				properties.Providers.Azure = r.providers.Azure
			}
			if hasAws {
				properties.Providers.Aws = r.providers.Aws
			}
		}
	}

	resource := &corerp.EnvironmentResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: properties,
	}

	err = client.CreateOrUpdateEnvironment(ctx, r.EnvironmentName, resource)
	if err != nil {
		return err
	}
	r.Output.LogInfo("Applications.Core/environments/%s created", r.EnvironmentName)

	return nil
}
