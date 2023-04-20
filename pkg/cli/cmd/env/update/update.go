// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package update

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"
)

const (
	envNotFoundErrMessage = "Environment does not exist. Please select a new environment and try again."
	azureScopeTemplate    = "/subscriptions/%s/resourceGroups/%s"
	awsScopeTemplate      = "/planes/aws/aws/accounts/%s/regions/%s"
)

// NewCommand creates an instance of the command and runner for the `rad env update` command.
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
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env update` command.
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
	azureSubId, err := cmd.Flags().GetString(commonflags.AzureSubscriptionIdFlag)
	if err != nil {
		return err
	}

	azureRgId, err := cmd.Flags().GetString(commonflags.AzureResourceGroupFlag)
	if err != nil {
		return err
	}
	if azureSubId != "" && azureRgId != "" {
		r.providers.Azure.Scope = to.Ptr(fmt.Sprintf(azureScopeTemplate, azureSubId, azureRgId))
	}

	r.clearEnvAzure, err = cmd.Flags().GetBool(commonflags.ClearEnvAzureFlag)
	if err != nil {
		return err
	}

	// TODO: Validate AWS scope components (https://github.com/project-radius/radius/issues/5155)
	// stsclient can be used to validate
	awsRegion, err := cmd.Flags().GetString(commonflags.AWSRegionFlag)
	if err != nil {
		return err
	}

	awsAccountId, err := cmd.Flags().GetString(commonflags.AWSAccountIdFlag)
	if err != nil {
		return err
	}
	if awsRegion != "" && awsAccountId != "" {
		r.providers.Aws.Scope = to.Ptr(fmt.Sprintf(awsScopeTemplate, awsAccountId, awsRegion))
	}

	r.clearEnvAws, err = cmd.Flags().GetBool(commonflags.ClearEnvAWSFlag)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env update` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.noFlagsSet {
		return nil
	}
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	env, err := client.GetEnvDetails(ctx, r.EnvName)
	if err != nil {
		if clients.Is404Error(err) {
			return &cli.FriendlyError{Message: envNotFoundErrMessage}
		}
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

	isEnvUpdated, err := client.CreateEnvironment(ctx, r.EnvName, v1.LocationGlobal, env.Properties)
	if err != nil || !isEnvUpdated {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to configure cloud provider scope to the environment %s: %s", r.EnvName, err.Error())}
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
