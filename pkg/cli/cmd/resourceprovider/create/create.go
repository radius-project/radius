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
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resourceprovider create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [resource provider namespace] [input]",
		Short: "Create or update a resource provider",
		Long: `Create or update a resource provider
		
Resource providers are the entities that implement resource types such as 'Applications.Core/containers'. Resource providers can be defined, registered, and unregistered by users.

Creating a resource provider defines new resource types that can be used in applications.

Input can be passed in using a file or inline JSON as the second argument. Prefix the input with '@' to indicate a file path.
`,
		Example: `
# Create a resource provider (from file)
rad resourceprovider create Applications.Example @/path/to/input.json

# Create a resource provider (inline)
rad resourceprovider create Applications.Example '{ ... }'`,
		Args: cobra.ExactArgs(2),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourceprovider create` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	ResourceProviderNamespace string
	ResourceProvider          *v20231001preview.ResourceProviderResource
}

// NewRunner creates an instance of the runner for the `rad resourceprovider create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resourceprovider create` command.
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

	r.ResourceProviderNamespace = args[0]
	r.ResourceProvider, err = readInput(args[1])
	if err != nil {
		return err
	}

	return nil
}

func readInput(arg string) (*v20231001preview.ResourceProviderResource, error) {
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

	resource := v20231001preview.ResourceProviderResource{}
	err := json.NewDecoder(strings.NewReader(string(bs))).Decode(&resource)
	if err != nil {
		return nil, clierrors.Message("Invalid input, could not be converted to a resource provider: %v", err)
	}

	return &resource, nil
}

// Run runs the `rad resourceprovider create` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	response, err := client.CreateOrUpdateResourceProvider(ctx, "local", r.ResourceProviderNamespace, r.ResourceProvider)
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, response, common.GetResourceProviderTableFormat())
	if err != nil {
		return err
	}

	return nil
}
