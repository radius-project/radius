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
	"strings"

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

## Set recipe packs to environment (--preview)
rad env update myenv --recipe-packs pack1,pack2
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	cmd.Flags().Bool(commonflags.ClearEnvAzureFlag, false, "Specify if azure provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvAWSFlag, false, "Specify if aws provider needs to be cleared on env")
	cmd.Flags().Bool(commonflags.ClearEnvKubernetesFlag, false, "Specify if kubernetes provider needs to be cleared on env (--preview)")
	cmd.Flags().StringSliceP("recipe-packs", "", []string{}, "Specify recipe packs to replace the environment's recipe pack list (--preview). Accepts comma-separated values.")
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
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
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

	recipePacks, err := cmd.Flags().GetStringSlice("recipe-packs")
	if err != nil {
		return err
	}

	r.recipePacks = normalizeRecipePacks(recipePacks)

	return nil
}

// normalizeRecipePacks splits comma-separated values, trims whitespace, and
// removes empty entries and duplicates while preserving the first-seen order.
// Deduplication avoids redundant referencedBy sync work and prevents server-side
// recipe pack conflict validation from failing on repeated entries.
func normalizeRecipePacks(recipepacks []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range recipepacks {
		for p := range strings.SplitSeq(value, ",") {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
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

	newRecipePacks := []*string{}

	// Update recipe packs if specified: replace the environment's recipe pack list,
	// keeping referencedBy on each recipe pack in sync.
	if len(r.recipePacks) > 0 {
		if len(env.Properties.RecipePacks) > 0 {
			r.Output.LogInfo("WARNING: The existing recipe pack list will be replaced with the specified packs.")
		}

		envID := *env.ID

		// Resolve all new recipe packs to full IDs.
		for _, recipePack := range r.recipePacks {
			var rClientFactory *corerpv20250801.ClientFactory
			recipePackID, err := resources.Parse(recipePack)
			// If the provided recipe pack value is an ID, parse its scope.
			if err == nil {
				rClientFactory, err = cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, recipePackID.RootScope())
				if err != nil {
					return err
				}
			} else {
				rClientFactory = r.RadiusCoreClientFactory
				scopeID, err := resources.ParseScope(r.Workspace.Scope)
				if err != nil {
					return err
				}

				recipePackID = scopeID.Append(resources.TypeSegment{
					Type: "Radius.Core/recipePacks",
					Name: recipePack,
				})
			}

			cfclient := rClientFactory.NewRecipePacksClient()

			_, err = cfclient.Get(ctx, recipePackID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
			if err != nil {
				return clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to set on the environment.", recipePack)
			}

			newRecipePacks = append(newRecipePacks, to.Ptr(recipePackID.String()))
		}

		err := syncRecipePackReferences(ctx, envID, env.Properties.RecipePacks, newRecipePacks, r.Workspace, r.RadiusCoreClientFactory)
		if err != nil {
			return err
		}

		// Replace the entire recipe packs list
		env.Properties.RecipePacks = newRecipePacks
	}

	_, err = envClient.CreateOrUpdate(ctx, r.EnvironmentName, env, &corerpv20250801.EnvironmentsClientCreateOrUpdateOptions{})
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to update environment %q.", r.EnvironmentName)
	}

	r.Output.LogInfo("Radius.Core/environments/%s updated", r.EnvironmentName)

	return nil
}

// syncRecipePackReferences updates the referencedBy field on recipe packs to reflect
// a new environment-to-pack assignment. When an environment is updated with a new list of recipe packs,
//
//	this function removes the environment from the referencedBy list of recipe packs being removed,
//
// and adds the environment to the referencedBy list of newly added recipe packs.
func syncRecipePackReferences(ctx context.Context, envID string, oldPackIDs []*string, newPackIDs []*string, workspace *workspaces.Workspace, defaultFactory *corerpv20250801.ClientFactory) error {
	oldPackIDSet := makePackIDSet(oldPackIDs)
	newPackIDSet := makePackIDSet(newPackIDs)
	packIDsToAdd := packIDsOnlyInFirst(newPackIDs, oldPackIDSet)
	packIDsToRemove := packIDsOnlyInFirst(oldPackIDs, newPackIDSet)
	clientsByScope := map[string]*corerpv20250801.RecipePacksClient{}

	// Remove this environment from referencedBy for packs being dropped.
	for _, oldPackIDStr := range packIDsToRemove {
		err := removeEnvReferenceFromRecipePack(ctx, envID, oldPackIDStr, workspace, defaultFactory, clientsByScope)
		if err != nil {
			return err
		}
	}

	// Add this environment to referencedBy for newly added packs.
	for _, newPackIDStr := range packIDsToAdd {
		err := addEnvReferenceToRecipePack(ctx, envID, newPackIDStr, workspace, defaultFactory, clientsByScope)
		if err != nil {
			return err
		}
	}

	return nil
}

func makePackIDSet(packIDs []*string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, packID := range packIDs {
		if packID == nil {
			continue
		}

		result[*packID] = struct{}{}
	}

	return result
}

func packIDsOnlyInFirst(first []*string, secondSet map[string]struct{}) []string {
	result := []string{}
	for _, packID := range first {
		if packID == nil {
			continue
		}

		if _, exists := secondSet[*packID]; exists {
			continue
		}

		result = append(result, *packID)
	}

	return result
}

func getRecipePacksClientForScope(
	ctx context.Context,
	rootScope string,
	workspace *workspaces.Workspace,
	defaultFactory *corerpv20250801.ClientFactory,
	clientsByScope map[string]*corerpv20250801.RecipePacksClient,
) (*corerpv20250801.RecipePacksClient, error) {
	if client, ok := clientsByScope[rootScope]; ok {
		return client, nil
	}

	factory := defaultFactory
	if rootScope != workspace.Scope {
		var err error
		factory, err = cmd.InitializeRadiusCoreClientFactory(ctx, workspace, rootScope)
		if err != nil {
			return nil, err
		}
	}

	client := factory.NewRecipePacksClient()
	clientsByScope[rootScope] = client

	return client, nil
}

func removeEnvReferenceFromRecipePack(
	ctx context.Context,
	envID string,
	packID string,
	workspace *workspaces.Workspace,
	defaultFactory *corerpv20250801.ClientFactory,
	clientsByScope map[string]*corerpv20250801.RecipePacksClient,
) error {
	resourceID, err := resources.Parse(packID)
	if err != nil {
		return err
	}

	packClient, err := getRecipePacksClientForScope(ctx, resourceID.RootScope(), workspace, defaultFactory, clientsByScope)
	if err != nil {
		return err
	}

	packResp, err := packClient.Get(ctx, resourceID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
	if clients.Is404Error(err) {
		return nil
	}
	if err != nil {
		return err
	}

	pack := packResp.RecipePackResource
	pack.SystemData = nil
	if pack.Properties == nil {
		pack.Properties = &corerpv20250801.RecipePackProperties{}
	}

	pack.Properties.ReferencedBy = removeReference(pack.Properties.ReferencedBy, envID)

	_, err = packClient.CreateOrUpdate(ctx, resourceID.Name(), pack, &corerpv20250801.RecipePacksClientCreateOrUpdateOptions{})
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to update recipe pack %q.", resourceID.Name())
	}

	return nil
}

func addEnvReferenceToRecipePack(
	ctx context.Context,
	envID string,
	packID string,
	workspace *workspaces.Workspace,
	defaultFactory *corerpv20250801.ClientFactory,
	clientsByScope map[string]*corerpv20250801.RecipePacksClient,
) error {
	resourceID, err := resources.Parse(packID)
	if err != nil {
		return err
	}

	packClient, err := getRecipePacksClientForScope(ctx, resourceID.RootScope(), workspace, defaultFactory, clientsByScope)
	if err != nil {
		return err
	}

	packResp, err := packClient.Get(ctx, resourceID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to add to the environment.", resourceID.String())
	}
	if err != nil {
		return err
	}

	pack := packResp.RecipePackResource
	pack.SystemData = nil
	if pack.Properties == nil {
		pack.Properties = &corerpv20250801.RecipePackProperties{}
	}

	if !refExists(pack.Properties.ReferencedBy, envID) {
		pack.Properties.ReferencedBy = append(pack.Properties.ReferencedBy, &envID)
	}

	_, err = packClient.CreateOrUpdate(ctx, resourceID.Name(), pack, &corerpv20250801.RecipePacksClientCreateOrUpdateOptions{})
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to update recipe pack %q.", resourceID.Name())
	}

	return nil
}

func removeReference(environmentRefs []*string, id string) []*string {
	result := []*string{}
	for _, ref := range environmentRefs {
		if ref != nil && *ref != id {
			result = append(result, ref)
		}
	}

	return result
}

func refExists(environmentRefs []*string, id string) bool {
	for _, r := range environmentRefs {
		if r != nil && *r == id {
			return true
		}
	}
	return false
}
