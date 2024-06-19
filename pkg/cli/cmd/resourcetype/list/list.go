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

package list

import (
	"context"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resourcetype list` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resource resource types",
		Long: `List resource resource types
		
Resource types are the entities that can be created and managed by Radius such as 'Applications.Core/containers'. Resource types can be configured using resource providers.`,
		Example: `
# List all resource types
rad resource type list`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourcetype list` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace
}

// NewRunner creates an instance of the runner for the `rad resourcetype list` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resourcetype list` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resourceprovider list` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceProviders, err := client.ListResourceProviders(ctx, "local")
	if err != nil {
		return err
	}

	resourceTypes := []common.ResourceType{}
	for _, resourceProvider := range resourceProviders {
		resourceTypes = append(resourceTypes, common.ResourceTypesForProvider(&resourceProvider)...)
	}

	slices.SortFunc(resourceTypes, func(a common.ResourceType, b common.ResourceType) int {
		return strings.Compare(a.Name, b.Name)
	})

	r.Output.WriteFormatted(r.Format, resourceTypes, common.GetResourceTypeTableFormat())

	return nil
}
