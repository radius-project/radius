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
	"fmt"

	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/spf13/cobra"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// NewCommand creates an instance of the command and runner for the `rad env create` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [envName]",
		Short: "Create a new Radius environment",
		Long: `Create a new Radius environment
Radius environments are prepared "landing zones" for Radius applications.
Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.`,
		Args:    cobra.MinimumNArgs(1),
		Example: `rad env create myenv`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddNamespaceFlag(cmd)
	cmd.Flags().Bool("skip-dev-recipes", false, "Use this flag to not use dev recipes")

	return cmd, runner
}

// Runner is the runner implementation for the `rad env create` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Workspace           *workspaces.Workspace
	EnvironmentName     string
	UCPResourceGroup    string
	Namespace           string
	ConnectionFactory   connections.Factory
	ConfigFileInterface framework.ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	NamespaceInterface  namespace.Interface
	SkipDevRecipes      bool
}

// NewRunner creates a new instance of the `rad env create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		NamespaceInterface:  factory.GetNamespaceInterface(),
	}
}

// Validate runs validation for the `rad env create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.SkipDevRecipes, err = cmd.Flags().GetBool("skip-dev-recipes")
	if err != nil {
		return err
	}

	r.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if r.Namespace == "" {
		r.Namespace = r.EnvironmentName
	}

	r.UCPResourceGroup, err = cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	if r.UCPResourceGroup == "" {
		// If no resource group specified and no default resource group
		if r.Workspace.Scope == "" {
			return &cli.FriendlyError{Message: "no resource group specified or set as default. Specify a resource group with '--group' and try again."}
		}
		// Use the default scope if no resource group provided
		scopeId, err := resources.Parse(r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.UCPResourceGroup = scopeId.FindScope(resources.ResourceGroupsSegment)
	}

	// If resource group specified but no scope set up in config.yaml
	if r.Workspace.Scope == "" {
		r.Workspace.Scope = "/planes/radius/local/resourcegroups/" + r.UCPResourceGroup
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	_, err = client.ShowUCPGroup(cmd.Context(), "radius", "local", r.UCPResourceGroup)
	if clients.Is404Error(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("Resource group %q could not be found.", r.UCPResourceGroup)}
	} else if err != nil {
		return err
	}

	err = r.NamespaceInterface.ValidateNamespace(cmd.Context(), r.Namespace)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env create` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Creating Environment...")

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	envProperties := &corerp.EnvironmentProperties{
		UseDevRecipes: to.Ptr(!r.SkipDevRecipes),
		Compute: &corerp.KubernetesCompute{
			Namespace: to.Ptr(r.Namespace),
		},
	}

	isEnvCreated, err := client.CreateEnvironment(ctx, r.EnvironmentName, v1.LocationGlobal, envProperties)
	if err != nil || !isEnvCreated {
		return err
	}
	r.Output.LogInfo("Successfully created environment %q in resource group %q", r.EnvironmentName, r.UCPResourceGroup)

	return nil
}
