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

package deploy

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad deploy` command.
//
// # Function Explanation
//
// NewCommand creates a new Cobra command and a Runner to deploy a Bicep or ARM template to a specified environment, with
// optional parameters. It also adds common flags to the command for workspace, resource group, environment name,
// application name and parameters.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "deploy [file]",
		Short: "Deploy a template",
		Long: `Deploy a Bicep or ARM template
	
	The deploy command compiles a Bicep or ARM template and deploys it to your default environment (unless otherwise specified).
		
	You can combine Radius types as as well as other types that are available in Bicep such as Azure resources. See
	the Radius documentation for information about describing your application and resources with Bicep.
	
	You can specify parameters using the '--parameter' flag ('-p' for short). Parameters can be passed as:
	
	- A file containing multiple parameters using the ARM JSON parameter format (see below)
	- A file containing a single value in JSON format
	- A key-value-pair passed in the command line
	
	When passing multiple parameters in a single file, use the format described here:
	
		https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files
	
	You can specify parameters using multiple sources. Parameters can be overridden based on the 
	order the are provided. Parameters appearing later in the argument list will override those defined earlier.
	`,
		Example: `
# deploy a Bicep template
rad deploy myapp.bicep

# deploy an ARM template (json)
rad deploy myapp.json

# deploy to a specific workspace
rad deploy myapp.bicep --workspace production

# deploy using a specific environment
rad deploy myapp.bicep --environment production

# deploy using a specific environment and resource group
rad deploy myapp.bicep --environment production --group mygroup

# specify a string parameter
rad deploy myapp.bicep --parameters version=latest


# specify a non-string parameter using a JSON file
rad deploy myapp.bicep --parameters configuration=@myfile.json


# specify many parameters using an ARM JSON parameter file
rad deploy myapp.bicep --parameters @myfile.json


# specify parameters from multiple sources
rad deploy myapp.bicep --parameters @myfile.json --parameters version=latest
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddParameterFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad deploy` command.
type Runner struct {
	Bicep             bicep.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Deploy            deploy.Interface
	Output            output.Interface

	ApplicationName string
	EnvironmentName string
	FilePath        string
	Parameters      map[string]map[string]any
	Workspace       *workspaces.Workspace
	Providers       *clients.Providers
}

// NewRunner creates a new instance of the `rad deploy` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Bicep:             factory.GetBicep(),
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Deploy:            factory.GetDeploy(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad deploy` command.
//
// # Function Explanation
//
// Validate validates the workspace, scope, environment name, application name, and parameters from the command
// line arguments and returns an error if any of these are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	r.Workspace = workspace

	// Allow --group to override the scope
	scope, err := cli.RequireScope(cmd, *workspace)
	if err != nil {
		return err
	}

	// We don't need to explicitly validate the existence of the scope, because we'll validate the existence
	// of the environment later. That will give an appropriate error message for the case where the group
	// does not exist.
	workspace.Scope = scope

	r.EnvironmentName, err = cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}

	// This might be empty, and that's fine!
	r.ApplicationName, err = cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}

	// Validate that the environment exists.
	// Right now we assume that every deployment uses a Radius environment.
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}
	env, err := client.GetEnvDetails(cmd.Context(), r.EnvironmentName)
	if clients.Is404Error(err) {
		return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` try again.", r.EnvironmentName, r.Workspace.Scope)
	} else if err != nil {
		return err
	}
	r.Providers = &clients.Providers{}
	r.Providers.Radius = &clients.RadiusProvider{}
	r.Providers.Radius.EnvironmentID = r.Workspace.Scope + "/providers/applications.core/environments/" + r.EnvironmentName
	if r.ApplicationName != "" {
		r.Providers.Radius.ApplicationID = r.Workspace.Scope + "/providers/applications.core/applications/" + r.ApplicationName
	}
	if env.Properties != nil && env.Properties.Providers != nil {

		if env.Properties.Providers.Aws != nil {
			r.Providers.AWS = &clients.AWSProvider{
				Scope: *env.Properties.Providers.Aws.Scope,
			}
		}
		if env.Properties.Providers.Azure != nil {
			r.Providers.Azure = &clients.AzureProvider{
				Scope: *env.Properties.Providers.Azure.Scope,
			}
		}
	}

	r.FilePath = args[0]

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: bicep.OSFileSystem{}}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad deploy` command.
//
// # Function Explanation
//
// Run deploys a Bicep template into an environment from a workspace, optionally creating an application if
// specified, and displays progress and completion messages. It returns an error if any of the operations fail.
func (r *Runner) Run(ctx context.Context) error {
	template, err := r.Bicep.PrepareTemplate(r.FilePath)
	if err != nil {
		return err
	}

	// Create application if specified. This supports the case where the application resource
	// is not specified in Bicep. Creating the application automatically helps us "bootstrap" in a new environment.
	if r.ApplicationName != "" {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}

		err = client.CreateApplicationIfNotFound(ctx, r.ApplicationName, v20220315privatepreview.ApplicationResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220315privatepreview.ApplicationProperties{
				Environment: &r.Workspace.Environment,
			},
		})
		if err != nil {
			return err
		}
	}

	progressText := ""
	if r.ApplicationName == "" {
		progressText = fmt.Sprintf(
			"Deploying template '%v' into environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress...", r.FilePath, r.EnvironmentName, r.Workspace.Name)
	} else {
		progressText = fmt.Sprintf(
			"Deploying template '%v' for application '%v' and environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress... ", r.FilePath, r.ApplicationName, r.EnvironmentName, r.Workspace.Name)
	}

	_, err = r.Deploy.DeployWithProgress(ctx, deploy.Options{
		ConnectionFactory: r.ConnectionFactory,
		Workspace:         *r.Workspace,
		Template:          template,
		Parameters:        r.Parameters,
		ProgressText:      progressText,
		CompletionText:    "Deployment Complete",
		Providers:         r.Providers,
	})
	if err != nil {
		return err
	}

	return nil
}
