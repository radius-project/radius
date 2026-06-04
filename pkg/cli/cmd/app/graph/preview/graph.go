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
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/app/graph"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// NewCommand creates an instance of the command and runner for the `rad app graph --preview` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Shows the application graph (preview)",
		Long:  `Shows the application graph for a Radius.Core application using the preview API surface.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show graph for specified application
rad app graph my-application --preview`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the preview `rad app graph` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	ApplicationName string
	Format          string
}

// NewRunner creates a new instance of the preview graph runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the preview graph command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Format, err = cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the preview `rad app graph` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	appClient := r.RadiusCoreClientFactory.NewApplicationsClient()

	// Fetch the application graph — GetGraph returns 404 if the application does not exist.
	graphResponse, err := appClient.GetGraph(ctx, r.Workspace.Scope, r.ApplicationName, corerpv20250801.GetGraphRequest{}, &corerpv20250801.ApplicationsClientGetGraphOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message("Application %q does not exist or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	switch r.Format {
	case output.FormatJson:
		return r.Output.WriteFormatted(r.Format, graphResponse.ApplicationGraphResponse, output.FormatterOptions{})
	default:
		d := display(graphResponse.Resources, r.ApplicationName)
		r.Output.LogInfo(d)
		return nil
	}
}

// display builds the formatted output for the application graph as text.
// This is a v20250801preview-specific version of the display function.
func display(applicationResources []*corerpv20250801.ApplicationGraphResource, applicationName string) string {
	containerType := "Applications.Core/containers"
	sort.Slice(applicationResources, func(i, j int) bool {
		if strings.EqualFold(*applicationResources[i].Type, containerType) !=
			strings.EqualFold(*applicationResources[j].Type, containerType) {
			return strings.EqualFold(*applicationResources[i].Type, containerType)
		}
		if *applicationResources[i].Type != *applicationResources[j].Type {
			return *applicationResources[i].Type < *applicationResources[j].Type
		}
		if *applicationResources[i].Name != *applicationResources[j].Name {
			return *applicationResources[i].Name < *applicationResources[j].Name
		}
		return *applicationResources[i].ID < *applicationResources[j].ID
	})

	out := &strings.Builder{}
	out.WriteString(fmt.Sprintf("Displaying application: %s\n\n", applicationName))

	if len(applicationResources) == 0 {
		out.WriteString("(empty)")
		out.WriteString("\n\n")
		return out.String()
	}

	for _, resource := range applicationResources {
		out.WriteString(fmt.Sprintf("Name: %s (%s)\n", *resource.Name, *resource.Type))

		if len(resource.Connections) == 0 {
			out.WriteString("Connections: (none)\n")
		} else {
			out.WriteString("Connections:\n")
			for _, connection := range resource.Connections {
				connectionID, err := resources.Parse(*connection.ID)
				if err != nil {
					out.WriteString(fmt.Sprintf("Error: %s\n", err.Error()))
					continue
				}
				connectionName := connectionID.Name()
				connectionType := connectionID.Type()

				if *connection.Direction == corerpv20250801.DirectionOutbound {
					out.WriteString(fmt.Sprintf("  %s -> %s (%s)\n", *resource.Name, connectionName, connectionType))
				} else {
					out.WriteString(fmt.Sprintf("  %s (%s) -> %s\n", connectionName, connectionType, *resource.Name))
				}
			}
		}

		if len(resource.OutputResources) == 0 {
			out.WriteString("Resources: (none)\n")
		} else {
			out.WriteString("Resources:\n")
			for _, outputResource := range resource.OutputResources {
				link := makeHyperlink(outputResource)
				if link == "" {
					out.WriteString(fmt.Sprintf("  %s (%s)\n", *outputResource.Name, *outputResource.Type))
				} else {
					out.WriteString(fmt.Sprintf("  %s (%s)\n", link, *outputResource.Type))
				}
			}
		}

		out.WriteString("\n")
	}

	return out.String()
}

func makeHyperlink(resource *corerpv20250801.ApplicationGraphOutputResource) string {
	return graph.MakeResourceHyperlink(*resource.ID, *resource.Name)
}
