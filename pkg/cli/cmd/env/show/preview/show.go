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
	"cmp"
	"context"
	"slices"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// NewCommand creates a new Cobra command and a Runner object to show environment details, with flags for workspace,
// resource group, environment name and output.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show environment details",
		Long:  `Show environment details. Shows the user's default environment by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show current environment
rad env show

# Show specified environment
rad env show my-env

# Show specified environment in a specified resource group
rad env show my-env --group my-env
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

type EnvRecipes struct {
	RecipePack     string
	ResourceType   string
	RecipeKind     string
	RecipeLocation string
}

// Runner is the runner implementation for the `rad env show` preview command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	Workspace               *workspaces.Workspace
	EnvironmentName         string
	Format                  string
	RadiusCoreClientFactory *corerpv20250801.ClientFactory
}

// NewRunner creates a new instance of the `rad env show` preview runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env show` preview command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
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

	r.Format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env show` preview command.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	envClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()

	resp, err := envClient.Get(ctx, r.EnvironmentName, &corerpv20250801.EnvironmentsClientGetOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message("The environment %q does not exist. Please select a new environment and try again.", r.EnvironmentName)
	} else if err != nil {
		return err
	}

	recipepackClient := r.RadiusCoreClientFactory.NewRecipePacksClient()
	envRecipes := []EnvRecipes{}
	for _, rp := range resp.EnvironmentResource.Properties.RecipePacks {
		ID, err := resources.Parse(*rp)
		if err != nil {
			return err
		}

		client := recipepackClient
		if ID.RootScope() != r.Workspace.Scope {
			factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, ID.RootScope())
			if err != nil {
				return err
			}

			client = factory.NewRecipePacksClient()
		}

		pack, err := client.Get(ctx, ID.Name(), &corerpv20250801.RecipePacksClientGetOptions{})
		if err != nil {
			return err
		}

		for resourceType, recipe := range pack.RecipePackResource.Properties.Recipes {
			envRecipes = append(envRecipes, EnvRecipes{
				RecipePack:     ID.Name(),
				ResourceType:   resourceType,
				RecipeKind:     string(*recipe.RecipeKind),
				RecipeLocation: *recipe.RecipeLocation,
			})
		}
	}

	// Sort for deterministic output
	slices.SortFunc(envRecipes, func(a, b EnvRecipes) int {
		if v := cmp.Compare(a.RecipePack, b.RecipePack); v != 0 {
			return v
		}
		return cmp.Compare(a.ResourceType, b.ResourceType)
	})

	err = r.Output.WriteFormatted(r.Format, resp.EnvironmentResource, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	r.Output.LogInfo("")
	err = r.Output.WriteFormatted(r.Format, envRecipes, objectformats.GetRecipesForEnvironmentTableFormat())
	if err != nil {
		return err
	}

	return nil
}
