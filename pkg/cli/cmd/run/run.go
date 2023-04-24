// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	deploycmd "github.com/project-radius/radius/pkg/cli/cmd/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes/logstream"
	"github.com/project-radius/radius/pkg/cli/kubernetes/portforward"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	cmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")

	return cmd, runner
}

// Runner is the runner implementation for the `rad run` command.
type Runner struct {
	deploycmd.Runner
	Logstream   logstream.Interface
	Portforward portforward.Interface
}

// NewRunner creates a new instance of the `rad run` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Runner:      *deploycmd.NewRunner(factory),
		Logstream:   factory.GetLogstream(),
		Portforward: factory.GetPortforward(),
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

	app, err := client.ShowApplication(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	namespace := ""
	appStatus := app.Properties.Status
	if appStatus != nil && appStatus.Compute != nil {
		kube, ok := appStatus.Compute.(*v20220315privatepreview.KubernetesCompute)
		if ok && kube.Namespace != nil {
			namespace = to.String(kube.Namespace)
		}
	}

	if namespace == "" {
		return &cli.FriendlyError{Message: "Only kubernetes runtimes are supported."}
	}

	// We start three background jobs and wait for them to complete.
	group, ctx := errgroup.WithContext(ctx)

	// 1. Display port-forward messages
	status := make(chan portforward.StatusMessage)
	group.Go(func() error {
		r.displayPortforwardMessages(status)
		return nil
	})

	// 2. Port-forward
	group.Go(func() error {
		return r.Portforward.Run(ctx, portforward.Options{
			ApplicationName: r.ApplicationName,
			Namespace:       namespace,
			KubeContext:     kubeContext,
			StatusChan:      status,
		})
	})

	// 3. Stream logs
	group.Go(func() error {
		return r.Logstream.Stream(ctx, logstream.Options{
			ApplicationName: r.ApplicationName,
			Namespace:       namespace,
			KubeContext:     kubeContext,

			// Right now we don't need an abstraction for this because we don't really
			// run the streaming logs in unit tests.
			Out: os.Stdout,
		})
	})

	err = group.Wait()

	// context.Canceled here means the user canceled.
	if errors.Is(err, context.Canceled) {
		return nil
	} else if err != nil {
		return err
	}

	return nil
}

func (r *Runner) displayPortforwardMessages(status <-chan portforward.StatusMessage) {
	regular := color.New(color.FgWhite)
	bold := color.New(color.FgHiWhite)

	for message := range status {
		// This format is used in functional tests to test the functionality. You will need to
		// update the tests if you make changes here.
		fmt.Printf("%s %s [port-forward] %s from localhost:%d -> ::%d\n", regular.Sprint(message.ReplicaName), bold.Sprint(message.ContainerName), message.Kind, message.LocalPort, message.RemotePort)
	}
}
