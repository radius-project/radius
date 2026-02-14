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

package show

import (
	"context"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates a new Cobra command and a Runner object to show recipe pack details, with flags for workspace,
// resource group and output.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show recipe pack details",
		Long:  `Show recipe pack details`,
		Args:  cobra.ExactArgs(1),
		Example: `

# Show specified recipe pack
rad recipe-show show my-recipe-pack

# Show specified recipe pack in a specified resource group
rad recipe-show show my-recipe-pack --group my-group
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlagWithPlainText(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe-pack show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface
	Format            string
	RecipePackName    string
}

// NewRunner creates a new instance of the `rad recipe-pack show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad recipe-pack show` command.
//
// Validate checks the request object for a workspace, scope, recipe pack name, and output format, and sets the
// corresponding fields in the Runner struct if they are found. If any of these fields are not found, an error is returned.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// Allow '--group' to override scope
	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	recipePackName, err := cli.RequireRecipePackNameArgs(cmd, args)
	if err != nil {
		return err
	}
	r.RecipePackName = recipePackName

	format, err := cli.RequireOutputAllowPlainText(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad recipe-pack show` command.
//

// Run attempts to retrieve recipe pack details from an ApplicationsManagementClient and write the details to an
// output in a specified format, returning an error if any of these operations fail.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	recipePack, err := client.GetRecipePack(ctx, r.RecipePackName)
	if clients.Is404Error(err) {
		return clierrors.Message("The recipe pack %q was not found or has been deleted.", r.RecipePackName)
	} else if err != nil {
		return err
	}

	if r.Format != "json" {
		err = r.Output.WriteFormatted(output.FormatTable, recipePack, objectformats.GetRecipePackTableFormat())
		if err != nil {
			return err
		}
		err = r.display(recipePack)
		if err != nil {
			return err
		}
	} else {
		err = r.Output.WriteFormatted(r.Format, recipePack, objectformats.GetRecipePackTableFormat())
		if err != nil {
			return err
		}
	}

	return nil
}
