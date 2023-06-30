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

package update

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"
)

const (
	envNotFoundErrMessageFmt = "The environment %q does not exist. Please select a new environment and try again."
	azureScopeTemplate       = "/subscriptions/%s/resourceGroups/%s"
	awsScopeTemplate         = "/planes/aws/aws/accounts/%s/regions/%s"
)

// NewCommand creates an instance of the command and runner for the `rad env update` command.
//
// # Function Explanation
//
// NewCommand creates a new Cobra command for updating an environment's configuration, such as adding or removing cloud
// providers, and returns a Runner to execute the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	runner.providers = &corerp.Providers{
		Azure: &corerp.ProvidersAzure{},
		Aws:   &corerp.ProvidersAws{},
	}

	cmd := &cobra.Command{
		Use:   "update [environment]",
		Short: "Update environment configuration",
		Long: `Update environment configuration
	
This command updates the configuration of an environment for properties that are able to be changed.
		
Properties that can be updated include:
- providers (Azure, AWS)
		  
All other properties require the environment to be deleted and recreated.
`,
		Args: cobra.ExactArgs(1),
		Example: `
## Add Azure cloud provider for deploying Azure resources
rad env update myenv --azure-subscription-id **** --azure-resource-group myrg

## Add AWS cloud provider for deploying AWS resources
rad env update myenv --aws-region us-west-2 --aws-account-id *****

## Remove Azure cloud provider
rad env update myenv --clear-azure

## Remove AWS cloud provider
rad env update myenv --clear-aws
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	cmd.Flags().Bool(commonflags.ClearEnvAzureFlag, false, "Specify if azure provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvAWSFlag, false, "Specify if aws provider needs to be cleared on env")
	commonflags.AddAzureScopeFlags(cmd)
	commonflags.AddAWSScopeFlags(cmd)
	commonflags.AddOutputFlag(cmd)
	//TODO: https://github.com/project-radius/radius/issues/5247
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env update` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	EnvName       string
	clearEnvAzure bool
	clearEnvAws   bool
	providers     *corerp.Providers
	noFlagsSet    bool
}

// NewRunner creates a new instance of the `rad env update` runner.
//
// # Function Explanation
//
// NewRunner creates a new Runner struct with the given factory's ConnectionFactory, ConfigHolder, and Output.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env update` command.
//
// # Function Explanation
//
// Validate checks the command flags and arguments for the required workspace, scope, environment name, Azure
// subscription ID, Azure resource group, AWS region, and AWS account ID, and sets the corresponding values in the Runner
// struct. If any of these values are not provided, an error is returned.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	if cmd.Flags().NFlag() == 0 {
		r.noFlagsSet = true
		return cmd.Help()
	}
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.EnvName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	// TODO: Validate Azure scope components (https://github.com/project-radius/radius/issues/5155)
	if cmd.Flags().Changed(commonflags.AzureSubscriptionIdFlag) || cmd.Flags().Changed(commonflags.AzureResourceGroupFlag) {
		azureSubId, err := cli.RequireAzureSubscriptionId(cmd)
		if err != nil {
			return err
		}

		azureRgId, err := cmd.Flags().GetString(commonflags.AzureResourceGroupFlag)
		if err != nil {
			return err
		}

		r.providers.Azure.Scope = to.Ptr(fmt.Sprintf(azureScopeTemplate, azureSubId, azureRgId))
	}

	r.clearEnvAzure, err = cmd.Flags().GetBool(commonflags.ClearEnvAzureFlag)
	if err != nil {
		return err
	}

	// TODO: Validate AWS scope components (https://github.com/project-radius/radius/issues/5155)
	// stsclient can be used to validate
	if cmd.Flags().Changed(commonflags.AWSRegionFlag) || cmd.Flags().Changed(commonflags.AWSAccountIdFlag) {
		awsRegion, err := cmd.Flags().GetString(commonflags.AWSRegionFlag)
		if err != nil {
			return err
		}

		awsAccountId, err := cmd.Flags().GetString(commonflags.AWSAccountIdFlag)
		if err != nil {
			return err
		}

		r.providers.Aws.Scope = to.Ptr(fmt.Sprintf(awsScopeTemplate, awsAccountId, awsRegion))
	}

	r.clearEnvAws, err = cmd.Flags().GetBool(commonflags.ClearEnvAWSFlag)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env update` command.
//
// # Function Explanation
//
// Run updates the environment with the given name with the given cloud provider scope and recipes. It returns an error
// if the environment does not exist or if the update fails.
func (r *Runner) Run(ctx context.Context) error {
	if r.noFlagsSet {
		return nil
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	env, err := client.GetEnvDetails(ctx, r.EnvName)
	if clients.Is404Error(err) {
		return clierrors.Message(envNotFoundErrMessageFmt, r.EnvName)
	} else if err != nil {
		return err
	}
	// only update azure provider info if user requires it.
	if r.clearEnvAzure && env.Properties.Providers != nil {
		env.Properties.Providers.Azure = nil
	} else if r.providers.Azure != nil && r.providers.Azure.Scope != nil {
		if env.Properties.Providers == nil {
			env.Properties.Providers = &corerp.Providers{}
		}
		env.Properties.Providers.Azure = r.providers.Azure
	}
	// only update aws provider info if user requires it.
	if r.clearEnvAws && env.Properties.Providers != nil {
		env.Properties.Providers.Aws = nil
	} else if r.providers.Aws != nil && r.providers.Aws.Scope != nil {
		if env.Properties.Providers == nil {
			env.Properties.Providers = &corerp.Providers{}
		}
		env.Properties.Providers.Aws = r.providers.Aws
	}

	r.Output.LogInfo("Updating Environment...")

	err = client.CreateEnvironment(ctx, r.EnvName, v1.LocationGlobal, env.Properties)
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to apply cloud provider scope to the environment %q.", r.EnvName)
	}

	recipeCount := 0
	if env.Properties.Recipes != nil {
		recipeCount = len(env.Properties.Recipes)
	}
	providerCount := 0
	if env.Properties.Providers != nil {
		if env.Properties.Providers.Azure != nil {
			providerCount++
		}
		if env.Properties.Providers.Aws != nil {
			providerCount++
		}
	}
	computeKind := ""
	if env.Properties.Compute != nil {
		computeKind = *env.Properties.Compute.GetEnvironmentCompute().Kind
	}
	obj := objectformats.OutputEnvObject{
		EnvName:     *env.Name,
		ComputeKind: computeKind,
		Recipes:     recipeCount,
		Providers:   providerCount,
	}

	err = r.Output.WriteFormatted("table", obj, objectformats.GetUpdateEnvironmentTableFormat())
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully updated environment %q.", r.EnvName)

	return nil
}
