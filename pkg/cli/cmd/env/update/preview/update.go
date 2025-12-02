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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	envNotFoundErrMessageFmt = "The environment %q does not exist. Please select a new environment and try again."
	awsScopeTemplate         = "/planes/aws/aws/accounts/%s/regions/%s"
)

// NewCommand creates an instance of the command and runner for the `rad env update` preview command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	runner.providers = &corerpv20250801.Providers{
		Azure:      &corerpv20250801.ProvidersAzure{},
		Aws:        &corerpv20250801.ProvidersAws{},
		Kubernetes: &corerpv20250801.ProvidersKubernetes{},
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

## Add Kubernetes cloud provider (preview)
rad env update myenv --kubernetes-namespace mynamespace

## Remove Kubernetes cloud provider (preview)
rad env update myenv --clear-kubernetes
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	cmd.Flags().Bool(commonflags.ClearEnvAzureFlag, false, "Specify if azure provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvAWSFlag, false, "Specify if aws provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvKubernetesFlag, false, "Specify if kubernetes provider needs to be cleared on env (preview)")
	cmd.Flags().StringArrayP("recipe-packs", "", []string{}, "Specify recipe packs to be added to the environment (preview)")
	commonflags.AddAzureScopeFlags(cmd)
	commonflags.AddAWSScopeFlags(cmd)
	commonflags.AddKubernetesScopeFlags(cmd)
	commonflags.AddOutputFlag(cmd)
	//TODO: https://github.com/radius-project/radius/issues/5247
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env update` preview command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	Workspace               *workspaces.Workspace
	Format                  string
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	EnvironmentName    string
	clearEnvAzure      bool
	clearEnvAws        bool
	clearEnvKubernetes bool
	providers          *corerpv20250801.Providers
	noFlagsSet         bool
	recipePacks        []string
}

// NewRunner creates a new instance of the `rad env update` preview runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env update` preview command.
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

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	// TODO: Validate Azure scope components (https://github.com/radius-project/radius/issues/5155)
	if cmd.Flags().Changed(commonflags.AzureSubscriptionIdFlag) || cmd.Flags().Changed(commonflags.AzureResourceGroupFlag) {
		azureSubId, err := cli.RequireAzureSubscriptionId(cmd)
		if err != nil {
			return err
		}

		azureRgId, err := cmd.Flags().GetString(commonflags.AzureResourceGroupFlag)
		if err != nil {
			return err
		}

		r.providers.Azure.SubscriptionID = to.Ptr(azureSubId)
		r.providers.Azure.ResourceGroupName = to.Ptr(azureRgId)
	}

	r.clearEnvAzure, err = cmd.Flags().GetBool(commonflags.ClearEnvAzureFlag)
	if err != nil {
		return err
	}

	// TODO: Validate AWS scope components (https://github.com/radius-project/radius/issues/5155)
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

	if cmd.Flags().Changed(commonflags.KubernetesNamespaceFlag) {
		k8sNamespace, err := cmd.Flags().GetString(commonflags.KubernetesNamespaceFlag)
		if err != nil {
			return err
		}

		r.providers.Kubernetes.Namespace = to.Ptr(k8sNamespace)
	}

	r.clearEnvKubernetes, err = cmd.Flags().GetBool(commonflags.ClearEnvKubernetesFlag)
	if err != nil {
		return err
	}

	r.recipePacks, err = cmd.Flags().GetStringArray("recipe-packs")
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env update` preview command.
func (r *Runner) Run(ctx context.Context) error {
	if r.noFlagsSet {
		return nil
	}

	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	envClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()

	// Get the current environment so we can update it.
	getResp, err := envClient.Get(ctx, r.EnvironmentName, &corerpv20250801.EnvironmentsClientGetOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message(envNotFoundErrMessageFmt, r.EnvironmentName)
	} else if err != nil {
		return err
	}

	// For now we just re-send the existing resource (placeholder for real updates).
	env := getResp.EnvironmentResource

	// SystemData is owned by the service; do not send it back on update.
	env.SystemData = nil

	// only update azure provider info if user requires it.
	if r.clearEnvAzure && env.Properties.Providers != nil {
		env.Properties.Providers.Azure = nil
	} else if r.providers.Azure != nil && (r.providers.Azure.SubscriptionID != nil || r.providers.Azure.ResourceGroupName != nil) {
		if env.Properties.Providers == nil {
			env.Properties.Providers = &corerpv20250801.Providers{}
		}
		env.Properties.Providers.Azure = r.providers.Azure
	}
	// only update aws provider info if user requires it.
	if r.clearEnvAws && env.Properties.Providers != nil {
		env.Properties.Providers.Aws = nil
	} else if r.providers.Aws != nil && r.providers.Aws.Scope != nil {
		if env.Properties.Providers == nil {
			env.Properties.Providers = &corerpv20250801.Providers{}
		}
		env.Properties.Providers.Aws = r.providers.Aws
	}

	// only update kubernetes provider info if user requires it.
	if r.clearEnvKubernetes && env.Properties.Providers != nil {
		env.Properties.Providers.Kubernetes = nil
	} else if r.providers.Kubernetes != nil && r.providers.Kubernetes.Namespace != nil {
		if env.Properties.Providers == nil {
			env.Properties.Providers = &corerpv20250801.Providers{}
		}
		env.Properties.Providers.Kubernetes = r.providers.Kubernetes
	}

	// add recipe packs if any
	if len(r.recipePacks) > 0 {
		if env.Properties.RecipePacks == nil {
			env.Properties.RecipePacks = []*string{}
		}

		for _, recipePack := range r.recipePacks {
			ID, err := resources.Parse(recipePack)
			rClientFactory := r.RadiusCoreClientFactory
			if err == nil {
				rClientFactory, err = cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, ID.RootScope())
				if err != nil {
					return err
				}

			} else {
				scopeID, err := resources.ParseScope(r.Workspace.Scope)
				if err != nil {
					return err
				}

				ID = scopeID.Append(resources.TypeSegment{
					Type: "Radius.Core/recipePacks",
					Name: recipePack,
				})
			}

			cfclient := rClientFactory.NewRecipePacksClient()
			_, err = cfclient.Get(ctx, ID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
			if err != nil {
				return clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to add to the environment.", recipePack)
			}

			if !recipePackExists(env.Properties.RecipePacks, ID.String()) {
				env.Properties.RecipePacks = append(env.Properties.RecipePacks, to.Ptr(ID.String()))
			}
		}
	}

	r.Output.LogInfo("Updating Environment...")
	_, err = envClient.CreateOrUpdate(ctx, r.EnvironmentName, env, &corerpv20250801.EnvironmentsClientCreateOrUpdateOptions{})
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to update environment %q.", r.EnvironmentName)
	}

	recipePackCount := 0
	if env.Properties.RecipePacks != nil {
		recipePackCount = len(env.Properties.RecipePacks)
	}
	providerCount := 0
	if env.Properties.Providers != nil {
		if env.Properties.Providers.Azure != nil {
			providerCount++
		}
		if env.Properties.Providers.Aws != nil {
			providerCount++
		}
		if env.Properties.Providers.Kubernetes != nil {
			providerCount++
		}
	}
	obj := environmentForDisplay{
		Name:        *env.Name,
		RecipePacks: recipePackCount,
		Providers:   providerCount,
	}

	err = r.Output.WriteFormatted("table", obj, environmentFormat())
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully updated environment %q.", r.EnvironmentName)

	return nil
}

func recipePackExists(packs []*string, id string) bool {
	for _, p := range packs {
		if p != nil && *p == id {
			return true
		}
	}
	return false
}
