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

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad install kubernetes` command and runner.
//

// NewCommand creates a new Cobra command and a new Runner object, which is used to install Radius onto a Kubernetes cluster.
// It also adds flags to the command for setting values, reinstalling, and specifying a Helm chart.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Installs Radius onto a kubernetes cluster",
		Long: `Install Radius in a Kubernetes cluster using the Radius Helm chart.
By default 'rad install kubernetes' will install Radius with the version matching the rad CLI version.

Radius will be installed in the 'radius-system' namespace. For more information visit https://docs.radapp.dev/concepts/architecture/

Overrides can be set by specifying Helm chart values with the '--set' flag. For more information visit https://docs.radapp.dev/operations/platforms/kubernetes/install/.
`,
		Example: `# Install Radius with default settings in current Kubernetes context
rad install kubernetes

# Install Radius with default settings in specified Kubernetes context
rad install kubernetes --kubecontext mycluster

# Install Radius with overrides in the current Kubernetes context
rad install kubernetes --set key=value
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)
	cmd.Flags().BoolVar(&runner.Reinstall, "reinstall", false, "Specify to force reinstallation of Radius")
	cmd.Flags().StringVar(&runner.Chart, "chart", "", "Specify a file path to a helm chart to install Radius from")
	cmd.Flags().StringArrayVar(&runner.Set, "set", []string{}, "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad install kubernetes` command.
type Runner struct {
	Helm   helm.Interface
	Output output.Interface

	KubeContext string
	Chart       string
	Reinstall   bool
	Set         []string
}

// NewRunner creates an instance of the runner for the `rad install kubernetes` command.
//

// NewRunner creates a new Runner struct with Helm and Output fields initialized with the HelmInterface and Output
// objects returned by the Factory's GetHelmInterface and GetOutput methods respectively.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:   factory.GetHelmInterface(),
		Output: factory.GetOutput(),
	}
}

// Validate runs validation for the `rad install kubernetes` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

// Run runs the `rad install kubernetes` command.
//

// Run checks if a Radius installation exists, and if it does, it either skips the installation or reinstalls it
// depending on the "Reinstall" flag. If no installation is found, it installs the version of Radius corresponding
// to the cli version. It then returns any errors that occur during the installation.
func (r *Runner) Run(ctx context.Context) error {
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall: r.Reinstall,
			ChartPath: r.Chart,
			SetArgs:   r.Set,
		},
	}

	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return err
	}

	if state.Installed && !r.Reinstall {
		r.Output.LogInfo("Found existing Radius installation. Use '--reinstall' to force reinstallation.")
		return nil
	}

	version := version.Version()
	if state.Installed {
		r.Output.LogInfo("Reinstalling Radius version %s to namespace: %s...", version, helm.RadiusSystemNamespace)
	} else {
		r.Output.LogInfo("Installing Radius version %s to namespace: %s...", version, helm.RadiusSystemNamespace)
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)
	_, err = r.Helm.InstallRadius(ctx, clusterOptions, r.KubeContext)
	if err != nil {
		return err
	}

	return nil
}
