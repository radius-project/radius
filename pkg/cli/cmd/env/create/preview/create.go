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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

// defaultKubernetesNamespace is used when the user does not specify a namespace.
// Recipes that deploy Kubernetes resources require one to target.
const defaultKubernetesNamespace = "default"

// NewCommand creates a new Cobra command and a Runner object to handle the `rad env create` command,
// and adds flags for environment name, workspace, resource group, Kubernetes namespace, and cloud provider configuration.
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
rad env create myenv --azure-subscription-id <subscription-id> --azure-resource-group <resource-group>

## Create environment with AWS cloud provider
rad env create myenv --aws-region <region> --aws-account-id <account-id>

## Create environment with a specific Kubernetes namespace
rad env create myenv --kubernetes-namespace mynamespace

## Create environment with recipe packs (--preview)
rad env create myenv --recipe-packs pack1,pack2
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
	commonflags.MarkNamespaceFlagDeprecated(cmd)
	cmd.MarkFlagsMutuallyExclusive(commonflags.KubernetesNamespaceFlag, commonflags.NamespaceFlag)
	cmd.Flags().StringSliceP("recipe-packs", "", []string{}, "Specify recipe packs to assign to the environment (--preview). Accepts comma-separated values.")

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

	recipePacks []string
	providers   *corerpv20250801.Providers
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

	// Initialize providers. Sub-structs are allocated lazily as flags are parsed.
	r.providers = &corerpv20250801.Providers{}

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

		r.providers.Azure = &corerpv20250801.ProvidersAzure{
			SubscriptionID:    new(azureSubId),
			ResourceGroupName: new(azureRgId),
		}
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

		r.providers.Aws = &corerpv20250801.ProvidersAws{
			Region:    new(awsRegion),
			AccountID: new(awsAccountId),
		}
	}

	// Kubernetes namespace: accept either --kubernetes-namespace or the legacy
	// --namespace alias (mutually exclusive). When neither is set, default to
	// "default" only if no other provider was configured — containers may
	// deploy as ACI (Azure) or otherwise target the configured provider, in
	// which case adding a Kubernetes namespace would be incorrect.
	k8sNamespace, set, err := commonflags.ResolveKubernetesNamespaceFlag(cmd)
	if err != nil {
		return err
	}
	shouldSetK8sNamespace := set
	if !set && r.providers.Azure == nil && r.providers.Aws == nil {
		k8sNamespace = defaultKubernetesNamespace
		shouldSetK8sNamespace = true
	}
	if shouldSetK8sNamespace {
		if err := prompt.ValidateKubernetesNamespace(k8sNamespace); err != nil {
			return clierrors.Message("Invalid Kubernetes namespace %q: %s", k8sNamespace, err.Error())
		}
		r.providers.Kubernetes = &corerpv20250801.ProvidersKubernetes{
			Namespace: new(k8sNamespace),
		}
	}

	recipePacks, err := cmd.Flags().GetStringSlice("recipe-packs")
	if err != nil {
		return err
	}
	r.recipePacks = normalizeRecipePacks(recipePacks)

	return nil
}

// Run runs the `rad env create --preview` command.
//
// Run creates a new Radius.Core environment with the recipe packs specified via
// --recipe-packs, or the default recipe pack when none are specified.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	// Resolve the recipe packs to assign to the environment. When the user
	// specifies --recipe-packs, those packs are used; otherwise the default
	// Radius recipe pack is created (if needed) and used.
	recipePackIDs, err := r.resolveRecipePacks(ctx)
	if err != nil {
		return err
	}

	properties := &corerpv20250801.EnvironmentProperties{
		RecipePacks: recipePackIDs,
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
	_, err = envClient.CreateOrUpdate(ctx, r.Workspace.Scope, r.EnvironmentName, *resource, nil)
	if err != nil {
		return err
	}

	// Keep referencedBy in sync on any user-specified recipe packs so each pack
	// records the environment that now references it. The default recipe pack
	// path does not maintain referencedBy.
	if len(r.recipePacks) > 0 {
		if err := r.addEnvironmentReferences(ctx, recipePackIDs); err != nil {
			return err
		}
	}

	r.Output.LogInfo("Radius.Core/environments/%s created", r.EnvironmentName)
	return nil
}

// resolveRecipePacks returns the list of recipe pack resource IDs to assign to
// the environment. When the user specifies recipe packs via --recipe-packs, each
// is resolved to a full resource ID and verified to exist. Otherwise the default
// Radius recipe pack is ensured and returned.
func (r *Runner) resolveRecipePacks(ctx context.Context) ([]*string, error) {
	if len(r.recipePacks) == 0 {
		return r.defaultRecipePack(ctx)
	}

	recipePackClient := r.RadiusCoreClientFactory.NewRecipePacksClient()

	recipePackIDs := make([]*string, 0, len(r.recipePacks))
	for _, recipePack := range r.recipePacks {
		recipePackID, err := resolveRecipePackID(recipePack, r.Workspace.Scope)
		if err != nil {
			return nil, err
		}

		_, err = recipePackClient.Get(ctx, recipePackID.RootScope(), recipePackID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
		if err != nil {
			return nil, clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to set on the environment.", recipePack)
		}

		recipePackIDs = append(recipePackIDs, to.Ptr(recipePackID.String()))
	}

	return recipePackIDs, nil
}

// defaultRecipePack ensures the default resource group and the default recipe
// pack exist and returns the default recipe pack ID. The default pack lives in
// the default scope regardless of the current workspace scope.
func (r *Runner) defaultRecipePack(ctx context.Context) ([]*string, error) {
	// Ensure the default resource group exists before creating recipe pack in it.
	mgmtClient, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return nil, err
	}
	if err := recipepack.EnsureDefaultResourceGroup(ctx, mgmtClient.CreateOrUpdateResourceGroup); err != nil {
		return nil, err
	}

	if r.DefaultScopeClientFactory == nil {
		defaultClientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace)
		if err != nil {
			return nil, err
		}
		r.DefaultScopeClientFactory = defaultClientFactory
	}

	recipePackClient := r.DefaultScopeClientFactory.NewRecipePacksClient()
	id, err := recipepack.GetOrCreateDefaultRecipePack(ctx, recipePackClient)
	if err != nil {
		return nil, err
	}

	return []*string{to.Ptr(id)}, nil
}

// addEnvironmentReferences records this environment in the referencedBy list of
// each specified recipe pack, keeping each pack's references in sync with the
// new assignment.
func (r *Runner) addEnvironmentReferences(ctx context.Context, recipePackIDs []*string) error {
	scopeID, err := resources.ParseScope(r.Workspace.Scope)
	if err != nil {
		return err
	}

	envID := scopeID.Append(resources.TypeSegment{
		Type: "Radius.Core/environments",
		Name: r.EnvironmentName,
	}).String()

	recipePackClient := r.RadiusCoreClientFactory.NewRecipePacksClient()
	for _, packID := range recipePackIDs {
		if packID == nil {
			continue
		}
		if err := addEnvReferenceToRecipePack(ctx, envID, *packID, recipePackClient); err != nil {
			return err
		}
	}

	return nil
}

// resolveRecipePackID resolves a recipe pack reference to a full resource ID.
// The reference may be a full resource ID or a bare name, in which case it is
// scoped to the environment's resource group.
func resolveRecipePackID(recipePack string, workspaceScope string) (resources.ID, error) {
	if recipePackID, err := resources.Parse(recipePack); err == nil {
		return recipePackID, nil
	}

	scopeID, err := resources.ParseScope(workspaceScope)
	if err != nil {
		return resources.ID{}, err
	}

	return scopeID.Append(resources.TypeSegment{
		Type: "Radius.Core/recipePacks",
		Name: recipePack,
	}), nil
}

// addEnvReferenceToRecipePack adds the environment ID to a recipe pack's
// referencedBy list if it is not already present.
func addEnvReferenceToRecipePack(ctx context.Context, envID string, packID string, client *corerpv20250801.RecipePacksClient) error {
	resourceID, err := resources.Parse(packID)
	if err != nil {
		return err
	}

	packResp, err := client.Get(ctx, resourceID.RootScope(), resourceID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
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

	_, err = client.CreateOrUpdate(ctx, resourceID.RootScope(), resourceID.Name(), pack, &corerpv20250801.RecipePacksClientCreateOrUpdateOptions{})
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to update recipe pack %q.", resourceID.Name())
	}

	return nil
}

// refExists reports whether id is present in the referencedBy list.
func refExists(refs []*string, id string) bool {
	for _, ref := range refs {
		if ref != nil && *ref == id {
			return true
		}
	}
	return false
}

// normalizeRecipePacks splits comma-separated values, trims whitespace, and
// removes empty entries and duplicates while preserving the first-seen order.
// Deduplication avoids redundant referencedBy sync work and prevents server-side
// recipe pack conflict validation from failing on repeated entries.
func normalizeRecipePacks(recipePacks []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range recipePacks {
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
