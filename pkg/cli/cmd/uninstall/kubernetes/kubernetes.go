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

package kubernetes

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad <fill in the blank>` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Uninstall Radius from a Kubernetes cluster",
		Long:  `Uninstall Radius from a Kubernetes cluster.`,
		Example: `# uninstall Radius from the current Kubernetes cluster
rad uninstall kubernetes

# uninstall Radius from a specific Kubernetes cluster based on the Kubeconfig context
rad uninstall kubernetes --kubecontext my-kubecontext`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad uninstall kubernetes` command.
type Runner struct {
	Helm   helm.Interface
	Output output.Interface

	KubeContext string
}

// NewRunner creates an instance of the runner for the `rad uninstall kubernetes` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:   factory.GetHelmInterface(),
		Output: factory.GetOutput(),
	}
}

// Validate runs validation for the `rad uninstall kubernetes` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

// Run runs the `rad uninstall kubernetes` command.
func (r *Runner) Run(ctx context.Context) error {
	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return err
	}
	if !state.Installed {
		r.Output.LogInfo("Radius is not installed on the Kubernetes cluster")
		return nil
	}

	r.Output.LogInfo("Uninstalling Radius...")
	err = r.Helm.UninstallRadius(ctx, r.KubeContext)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Radius was uninstalled successfully. Any existing data will be retained for future installations. Local configuration is also retained. Use the `rad workspace` command if updates are needed to your configuration.")
	return nil
}
