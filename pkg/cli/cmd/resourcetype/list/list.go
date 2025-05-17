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
	"golang.org/x/exp/maps"
)

// NewCommand creates an instance of the `rad resource-type list` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resource resource types",
		Long: `List resource resource types
		
Resource types are the entities that can be created and managed by Radius such as 'Applications.Core/containers'. Each resource type can define multiple API versions, and each API version defines a schema that resource instances conform to. Resource types can be configured using resource providers.`,
		Example: `
# List all resource types
rad resource-type list`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resource-type list` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace
}

// NewRunner creates an instance of the runner for the `rad resource-type list` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource-type list` command.
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

// Run runs the `rad resource-type list` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceProviders, err := client.ListResourceProviderSummaries(ctx, "local")
	if err != nil {
		return err
	}

	resourceTypes := []common.ResourceTypeListOutputFormat{}
	for _, resourceProvider := range resourceProviders {

		for _, rt := range common.ResourceTypesForProvider(&resourceProvider) {
			resourceType := common.ResourceTypeListOutputFormat{
				ResourceType:   rt,
				APIVersionList: maps.Keys(rt.APIVersions),
			}
			resourceTypes = append(resourceTypes, resourceType)
		}
	}

	slices.SortFunc(resourceTypes, func(a common.ResourceTypeListOutputFormat, b common.ResourceTypeListOutputFormat) int {
		return strings.Compare(a.Name, b.Name)
	})

	err = r.Output.WriteFormatted(r.Format, resourceTypes, common.GetResourceTypeTableFormat())
	if err != nil {
		return err
	}

	return nil
}
