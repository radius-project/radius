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

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	envNotFoundErrMessageFmt = "The environment %q does not exist. Please select a new environment and try again."
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
	cmd.Flags().Bool(commonflags.ClearEnvKubernetesFlag, false, "Specify if kubernetes provider needs to be cleared on env (--preview)")
	cmd.Flags().StringArrayP("recipe-packs", "", []string{}, "Specify recipe packs to be added to the environment (--preview)")
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

		r.providers.Azure.SubscriptionID = new(azureSubId)
		r.providers.Azure.ResourceGroupName = new(azureRgId)
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

		r.providers.Aws.Region = new(awsRegion)
		r.providers.Aws.AccountID = new(awsAccountId)
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

		r.providers.Kubernetes.Namespace = new(k8sNamespace)
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
	} else if r.providers.Aws != nil && (r.providers.Aws.AccountID != nil && r.providers.Aws.Region != nil) {
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

	// Update recipe packs if specified: replace the environment's recipe pack list,
	// keeping referencedBy on each recipe pack in sync.
	if len(r.recipePacks) > 0 {
		envID := *env.ID

		// Resolve and validate all new recipe packs (fetch each once).
		newPacks := []resolvedPack{}
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
			packResp, err := cfclient.Get(ctx, ID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
			if err != nil {
				return clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to add to the environment.", recipePack)
			}

			pack := packResp.RecipePackResource
			pack.SystemData = nil
			newPacks = append(newPacks, resolvedPack{id: ID, client: cfclient, pack: pack})
		}

		newPackIDs, err := syncRecipePackReferences(ctx, envID, env.Properties.RecipePacks, newPacks, r.Workspace, r.RadiusCoreClientFactory)
		if err != nil {
			return err
		}
		env.Properties.RecipePacks = newPackIDs
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

type resolvedPack struct {
	id     resources.ID
	client *corerpv20250801.RecipePacksClient
	pack   corerpv20250801.RecipePackResource
}

// syncRecipePackReferences updates the referencedBy field on recipe packs to reflect
// a new environment-to-pack assignment. Only packs whose referencedBy actually changes
// (dropped or newly added) are written to.
func syncRecipePackReferences(ctx context.Context, envID string, oldPackIDs []*string, newPacks []resolvedPack, workspace *workspaces.Workspace, defaultFactory *corerpv20250801.ClientFactory) ([]*string, error) {
	oldPackIDSet := map[string]struct{}{}
	for _, p := range oldPackIDs {
		oldPackIDSet[*p] = struct{}{}
	}

	newPackIDSet := map[string]struct{}{}
	for _, np := range newPacks {
		newPackIDSet[np.id.String()] = struct{}{}
	}

	// Remove this environment from referencedBy for packs being dropped.
	for _, oldPackIDStr := range oldPackIDs {
		if _, stillReferenced := newPackIDSet[*oldPackIDStr]; stillReferenced {
			continue
		}

		oldID, err := resources.Parse(*oldPackIDStr)
		if err != nil {
			return nil, err
		}

		factory := defaultFactory
		if oldID.RootScope() != workspace.Scope {
			factory, err = cmd.InitializeRadiusCoreClientFactory(ctx, workspace, oldID.RootScope())
			if err != nil {
				return nil, err
			}
		}

		oldPackClient := factory.NewRecipePacksClient()
		oldPackResp, err := oldPackClient.Get(ctx, oldID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
		if clients.Is404Error(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		oldPack := oldPackResp.RecipePackResource
		oldPack.SystemData = nil
		filtered := []*string{}
		for _, ref := range oldPack.Properties.ReferencedBy {
			if ref != nil && *ref != envID {
				filtered = append(filtered, ref)
			}
		}
		oldPack.Properties.ReferencedBy = filtered

		_, err = oldPackClient.CreateOrUpdate(ctx, oldID.Name(), oldPack, &corerpv20250801.RecipePacksClientCreateOrUpdateOptions{})
		if err != nil {
			return nil, clierrors.MessageWithCause(err, "Failed to update recipe pack %q.", oldID.Name())
		}
	}

	// Add this environment to referencedBy for newly added packs.
	newPackIDs := []*string{}
	for i := range newPacks {
		np := &newPacks[i]
		idStr := np.id.String()

		if _, wasReferenced := oldPackIDSet[idStr]; !wasReferenced {
			if !refExists(np.pack.Properties.ReferencedBy, envID) {
				np.pack.Properties.ReferencedBy = append(np.pack.Properties.ReferencedBy, &envID)
			}
			_, err := np.client.CreateOrUpdate(ctx, np.id.Name(), np.pack, &corerpv20250801.RecipePacksClientCreateOrUpdateOptions{})
			if err != nil {
				return nil, clierrors.MessageWithCause(err, "Failed to update recipe pack %q.", np.id.Name())
			}
		}

		newPackIDs = append(newPackIDs, &idStr)
	}

	return newPackIDs, nil
}

func refExists(refs []*string, id string) bool {
	for _, r := range refs {
		if r != nil && *r == id {
			return true
		}
	}
	return false
}
