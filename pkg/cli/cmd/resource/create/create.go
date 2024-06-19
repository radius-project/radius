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

package create

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resource create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [resource type] [name] [input]",
		Short: "Create or update a resource",
		Long: `Create or update a resource
		
Resources are the entities are the primary entities that make up applactions.

Input can be passed in using a file or inline JSON as the second argument. Prefix the input with '@' to indicate a file path.
`,
		Example: `
# Create a resource (from file)
rad resource create 'Applications.Core/containers' mycontainer @/path/to/input.json

# Create a resource (inline)
rad resource create 'Applications.Core/containers' mycontainer '{ ... }'`,
		Args: cobra.ExactArgs(3),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resource create` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	ResourceType string
	ResourceName string
	Resource     *generated.GenericResource
}

// NewRunner creates an instance of the runner for the `rad resource create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
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

	r.ResourceType = args[0]
	r.ResourceName = args[1]
	r.Resource, err = readInput(args[2])
	if err != nil {
		return err
	}

	return nil
}

func readInput(arg string) (*generated.GenericResource, error) {
	// Input could either be a file or inline JSON. The @ prefix will help us determine which one it is.
	var bs []byte
	if strings.HasPrefix(arg, "@") {
		inputFile := strings.TrimPrefix(arg, "@")

		var err error
		bs, err = os.ReadFile(inputFile)
		if err != nil {
			return nil, clierrors.Message("Failed to read input file: %v", err)
		}
	} else {
		bs = []byte(arg)
	}

	decoder := json.NewDecoder(strings.NewReader(string(bs)))
	decoder.DisallowUnknownFields()

	resource := generated.GenericResource{}
	err := decoder.Decode(&resource)
	if err != nil {
		return nil, clierrors.Message("Invalid input, could not be converted to a resource: %v", err)
	}

	return &resource, nil
}

// Run runs the `rad resource create` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	response, err := client.CreateOrUpdateResource(ctx, r.ResourceType, r.ResourceName, r.Resource)
	if err != nil {
		return err
	}

	r.Output.WriteFormatted(r.Format, response, common.GetResourceProviderTableFormat())

	return nil
}
