// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package run

import (
	"context"
	"errors"
	"os"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	deploycmd "github.com/project-radius/radius/pkg/cli/cmd/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes/logstream"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad run` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "run [file]",
		Short: "Run an application",
		Long: `Run an application specified by a Bicep or ARM template
	
	The run command compiles a Bicep or ARM template and runs it in your default environment (unless otherwise specified).
		
	The run command accepts the same parameters as the 'rad deploy' command. See the 'rad deploy' help for more information.
	`,
		Example: `
# Run app.bicep
rad run app.bicep

# Run in a specific environment
rad run app.bicep --environment prod

# Run app.bicep and specify a string parameter
rad run app.bicep --parameters version=latest

# Run app.bicep and specify parameters from multiple sources
rad run app.bicep --parameters @myfile.json --parameters version=latest
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	cmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")

	return cmd, runner
}

// Runner is the runner implementation for the `rad run` command.
type Runner struct {
	deploycmd.Runner
	Logstream logstream.Interface
}

// NewRunner creates a new instance of the `rad run` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Runner:    *deploycmd.NewRunner(factory),
		Logstream: factory.GetLogstream(),
	}
}

// Validate runs validation for the `rad run` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	err := r.Runner.Validate(cmd, args)
	if err != nil {
		return err
	}

	// In addition to the deployment validations, this command requires an application name
	if r.ApplicationName == "" {
		return &cli.FriendlyError{Message: "No application was specified. Use --application to specify the application name."}
	}

	return nil
}

// Run runs the `rad run` command.
func (r *Runner) Run(ctx context.Context) error {
	// Call into base first to deploy, and then set up port-forwards and logs.
	err := r.Runner.Run(ctx)
	if err != nil {
		return err
	}

	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return nil
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Starting log stream...")
	r.Output.LogInfo("")

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return nil
	}

	// We don't expect an error here because we already deployed to the environment
	environment, err := client.GetEnvDetails(ctx, r.EnvironmentName)
	if err != nil {
		return err
	}

	namespace := ""
	switch compute := environment.Properties.Compute.(type) {
	case *v20220315privatepreview.KubernetesCompute:
		namespace = *compute.Namespace
	default:
		return &cli.FriendlyError{Message: "Only kubernetes runtimes are supported."}
	}

	err = r.Logstream.Stream(ctx, logstream.Options{
		ApplicationName: r.ApplicationName,
		Namespace:       namespace,
		KubeContext:     kubeContext,

		// Right now we don't need an abstraction for this because we don't really
		// run the streaming logs in unit tests.
		Out: os.Stdout,
	})

	// context.Canceled here means the user canceled.
	if errors.Is(err, context.Canceled) {
		return nil
	} else if err != nil {
		return err
	}

	return nil
}
