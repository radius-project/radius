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
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"
)

const (
	setAndClearErrMessage = "Cannot set and clear env provider"
	envNotFoundErrMessage = "Env Creation required before udpation"
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
		Use:   "update [envName]",
		Short: "Updates configurable environment details.",
		Long:  `Updates configurable environment details. provider on env is one example`,
		Args:  cobra.MinimumNArgs(1),
		Example: `
# Update azure provider on enviroment
# When setting azure provider, subscriptionId and resourcegroup flags are required
rad env update my-env --set-azure-provider --subscription-id='subId' --resourcegroup='rgId'

# Update aws provider on environment
# When setting aws provider, region and accountId flags are required
rad env update my-env --set-aws-provider --region='us-west-2' --accountId='aId'
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddAzureScopeFlags(cmd)
	commonflags.AddAWSScopeFlags(cmd)
	commonflags.AddOutputFlag(cmd)

	cmd.Flags().Bool(commonflags.ClearEnvAzureFlag, false, "Specify if azure provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvAWSFlag, false, "Specify if aws provider needs to be cleared on env")

	return cmd, runner
}

// Runner is the runner implementation for the `rad env update` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	envName       string
	setEnvAzure   bool
	setEnvAws     bool
	clearEnvAzure bool
	clearEnvAws   bool
	providers     *corerp.Providers
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
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.envName = args[0]

	r.setEnvAzure, err = cmd.Flags().GetBool(commonflags.SetEnvAzureFlag)
	if err != nil {
		return err
	}

	r.clearEnvAzure, err = cmd.Flags().GetBool(commonflags.ClearEnvAzureFlag)
	if err != nil {
		return err
	}

	if r.setEnvAzure && r.clearEnvAzure {
		return &cli.FriendlyError{Message: setAndClearErrMessage}
	}

	if r.setEnvAzure {
		// TODO: Validate Azure scope components (https://github.com/project-radius/radius/issues/5155)
		azureSubId, err := cmd.Flags().GetString(commonflags.AzureSubscriptionIdFlag)
		if err != nil {
			return err
		}

		azureResourceGroup, err := cmd.Flags().GetString(commonflags.AzureResourceGroupFlag)
		if err != nil {
			return err
		}
		r.providers.Azure.Scope = to.Ptr(fmt.Sprintf(azureScopeTemplate, azureSubId, azureResourceGroup))
	}

	if r.clearEnvAws {
		r.providers.Aws = nil
	}

	r.setEnvAws, err = cmd.Flags().GetBool(commonflags.SetEnvAWSFlag)
	if err != nil {
		return err
	}

	r.clearEnvAws, err = cmd.Flags().GetBool(commonflags.ClearEnvAWSFlag)
	if err != nil {
		return err
	}

	if r.setEnvAws && r.clearEnvAws {
		return &cli.FriendlyError{Message: setAndClearErrMessage}
	}

	if r.setEnvAws {
		// TODO: Validate AWS scope components (https://github.com/project-radius/radius/issues/5155)
		// stsclient can be used to validate
		awsRegion, err := cmd.Flags().GetString(commonflags.AWSRegionFlag)
		if err != nil {
			return err
		}

		awsAccount, err := cmd.Flags().GetString(commonflags.AWSAccountIdFlag)
		if err != nil {
			return err
		}
		r.providers.Aws.Scope = to.Ptr(fmt.Sprintf(awsScopeTemplate, awsAccount, awsRegion))

	}

	return nil
}

// Run runs the `rad env update` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Updating Environment...")

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	env, err := client.GetEnvDetails(ctx, r.envName)
	if err != nil {
		if clients.Is404Error(err) {
			return &cli.FriendlyError{Message: envNotFoundErrMessage}
		}
		return err
	}
	providers := &corerp.Providers{}
	//in case provider already exists, fetch it on local.
	if env.Properties.Providers != nil {
		providers = env.Properties.Providers
	}
	// only update azure provider info if user requires it.
	if r.setEnvAzure || r.clearEnvAzure {
		providers.Azure = r.providers.Azure
	}
	// only update aws provider info if user requires it.
	if r.setEnvAws || r.clearEnvAws {
		providers.Aws = r.providers.Aws
	}

	var namespace string
	compute, ok := env.Properties.Compute.(*corerp.KubernetesCompute)
	if ok {
		namespace = *compute.Namespace
	} else {
		namespace = r.envName

	}

	isEnvUpdated, err := client.CreateEnvironment(ctx, r.envName, v1.LocationGlobal, namespace, "Kubernetes", "", map[string]*corerp.EnvironmentRecipeProperties{}, providers, *env.Properties.UseDevRecipes)
	// In case of 304 (env not updated but no error), return nil
	if err != nil || !isEnvUpdated {
		return err
	}

	env.Properties.Providers = providers
	err = r.Output.WriteFormatted("table", env, objectformats.GetEnvironmentTableFormat())
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully updated environment %q.", r.envName)

	return nil
}
