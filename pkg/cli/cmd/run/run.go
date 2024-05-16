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

package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	deploycmd "github.com/radius-project/radius/pkg/cli/cmd/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/kubernetes/logstream"
	"github.com/radius-project/radius/pkg/cli/kubernetes/portforward"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sclient "k8s.io/client-go/kubernetes"
	k8srest "k8s.io/client-go/rest"
)

const (
	radiusSystemNamespace = "radius-system"
	dashboardLabelName    = "dashboard"
	dashboardLabelPartOf  = "radius"
)

// NewCommand creates an instance of the command and runner for the `rad run` command.
//

// NewCommand creates a new Cobra command that can be used to run an application specified by a Bicep or ARM template,
// port-forward container ports and stream container logs to a user's terminal, and accepts the same parameters as the 'rad
//
//	deploy' command. It returns an error if the command is not run with exactly one argument.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "run [file]",
		Short: "Run an application",
		Long: `Run an application specified by a Bicep or ARM template
	
The run command compiles a Bicep or ARM template and runs it in your default environment (unless otherwise specified). It also automatically port-forwards container ports and streams container logs to a user's terminal.
		
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
	Logstream            logstream.Interface
	Portforward          portforward.Interface
	kubernetesClient     k8sclient.Interface
	kubernetesRESTConfig *k8srest.Config
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
//

// The Validate function performs additional validations on the deployment and requires an application name, returning an
// error if one is not specified.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	err := r.Runner.Validate(cmd, args)
	if err != nil {
		return err
	}

	// In addition to the deployment validations, this command requires an application name
	if r.ApplicationName == "" {
		return clierrors.Message("No application was specified. Use --application to specify the application name.")
	}

	return nil
}

// Run runs the `rad run` command.
//

// Run starts port-forwarding and log streaming for a given application in a given Kubernetes context, and
// returns an error if any of the operations fail.
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

	app, err := client.GetApplication(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	namespace := ""
	appStatus := app.Properties.Status
	if appStatus != nil && appStatus.Compute != nil {
		kube, ok := appStatus.Compute.(*v20231001preview.KubernetesCompute)
		if ok && kube.Namespace != nil {
			namespace = to.String(kube.Namespace)
		}
	}

	if namespace == "" {
		return clierrors.Message("Only kubernetes runtimes are supported.")
	}

	applicationSelector, err := portforward.CreateLabelSelectorForApplication(r.ApplicationName)
	if err != nil {
		return err
	}

	dashboardSelector, err := portforward.CreateLabelSelectorForDashboard()
	if err != nil {
		return err
	}

	if r.kubernetesClient == nil && r.kubernetesRESTConfig == nil {
		kubernetesClient, kubernetesRESTConfig, err := kubernetes.NewClientset(kubeContext)
		if err != nil {
			return err
		}

		r.kubernetesClient = kubernetesClient
		r.kubernetesRESTConfig = kubernetesRESTConfig
	}

	// We start some background jobs and wait for them to complete.
	group, ctx := errgroup.WithContext(ctx)

	// Display port-forward messages for application
	applicationStatusChan := make(chan portforward.StatusMessage)
	group.Go(func() error {
		r.displayPortforwardMessages(applicationStatusChan)
		return nil
	})

	// Port-forward application
	group.Go(func() error {
		return r.Portforward.Run(ctx, portforward.Options{
			LabelSelector: applicationSelector,
			Namespace:     namespace,
			KubeContext:   kubeContext,
			StatusChan:    applicationStatusChan,
			Out:           os.Stdout,
			Client:        r.kubernetesClient,
			RESTConfig:    r.kubernetesRESTConfig,
		})
	})

	if dashboardDeploymentExists(ctx, r.kubernetesClient, dashboardSelector) {
		// Display port-forward messages for dashboard
		dashboardStatusChan := make(chan portforward.StatusMessage)
		group.Go(func() error {
			r.displayPortforwardMessages(dashboardStatusChan)
			return nil
		})

		// Port-forward dashboard
		group.Go(func() error {
			return r.Portforward.Run(ctx, portforward.Options{
				LabelSelector: dashboardSelector,
				Namespace:     radiusSystemNamespace,
				KubeContext:   kubeContext,
				StatusChan:    dashboardStatusChan,
				Out:           os.Stdout,
				Client:        r.kubernetesClient,
				RESTConfig:    r.kubernetesRESTConfig,
			})
		})
	} else {
		fmt.Println("Radius Dashboard not found, please see https://docs.radapp.io/guides/tooling/dashboard for more information")
	}

	// Stream logs
	group.Go(func() error {
		return r.Logstream.Stream(ctx, logstream.Options{
			ApplicationName: r.ApplicationName,
			Namespace:       namespace,
			KubeClient:      r.kubernetesClient,

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

// dashboardDeploymentExists checks if a dashboard deployment exists in the given Kubernetes context.
func dashboardDeploymentExists(ctx context.Context, kubernetesClient k8sclient.Interface, dashboardLabelSelector labels.Selector) bool {
	deployments := kubernetesClient.AppsV1().Deployments(radiusSystemNamespace)
	listOptions := metav1.ListOptions{LabelSelector: dashboardLabelSelector.String()}

	// List all deployments that match the label selector
	labelledDeployments, err := deployments.List(ctx, listOptions)
	if err != nil {
		return false
	}

	// If there are any deployments that match the dashboard label selector, return true.
	// Otherwise, return false.
	return len(labelledDeployments.Items) != 0
}
