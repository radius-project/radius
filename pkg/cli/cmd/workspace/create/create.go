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
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad workspace create` command.
//

// NewCommand creates a new Cobra command and a Runner to handle the command's logic, and adds flags to the command for
// workspace, resource group, environment name, force and context.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [workspaceType] [workspaceName]",
		Short: "Create a workspace",
		Long: `Create a workspace.
		
Available workspaceTypes: kubernetes

Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file. 

You can easily define and switch between workspaces to deploy and manage applications across local, test, and production environments.`,
		Args: ValidateArgs(),
		Example: `
# Create a workspace with name 'myworkspace' and kubernetes context 'aks'
rad workspace create kubernetes myworkspace --context aks
# Create a workspace with name of current kubernetes context in current kubernetes context
rad workspace create kubernetes`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	cmd.Flags().StringP("context", "c", "", "the Kubernetes context to use, will use the default if unset")

	return cmd, runner
}

// EnvironmentValidator validates that the environment named envName exists in the
// scope of ws, and returns the fully-qualified environment resource ID to be persisted
// on the workspace. mgmtClient is the management client already created for the workspace
// scope; implementations are free to ignore it (e.g. preview implementations that talk
// to the Radius.Core resource provider directly).
type EnvironmentValidator func(ctx context.Context, ws *workspaces.Workspace, mgmtClient clients.ApplicationsManagementClient, envName string) (envID string, err error)

// Runner is the runner implementation for the `rad workspace create` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	ConnectionFactory   connections.Factory
	Workspace           *workspaces.Workspace
	Force               bool
	ConfigFileInterface framework.ConfigFileInterface
	Output              output.Interface
	HelmInterface       helm.Interface
	KubernetesInterface kubernetes.Interface

	// EnvironmentValidator is invoked to validate that the requested environment exists
	// and to construct the environment resource ID stored on the workspace. When nil,
	// the legacy Applications.Core/environments validator is used.
	EnvironmentValidator EnvironmentValidator
}

// NewRunner creates a new instance of the `rad workspace create` runner.
func NewRunner(factory framework.Factory) *Runner {
	r := &Runner{
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigHolder:        factory.GetConfigHolder(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		Output:              factory.GetOutput(),
		HelmInterface:       factory.GetHelmInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
	}
	r.EnvironmentValidator = ValidateApplicationsCoreEnvironment
	return r
}

// ValidateApplicationsCoreEnvironment is the default EnvironmentValidator. It targets
// Applications.Core/environments via the management client.
func ValidateApplicationsCoreEnvironment(ctx context.Context, ws *workspaces.Workspace, mgmtClient clients.ApplicationsManagementClient, envName string) (string, error) {
	envID := ws.Scope + "/providers/" + datamodel.EnvironmentResourceType + "/" + envName
	if _, err := mgmtClient.GetEnvironment(ctx, envName); err != nil {
		if clients.Is404Error(err) {
			return "", clierrors.Message("The environment %q does not exist. Run `rad env create` and try again.", envID)
		}
		return "", clierrors.MessageWithCause(err, "Failed to get environment %q.", envID)
	}
	return envID, nil
}

// Validate runs validation for the `rad workspace create` command.
//

// Validate checks if the given workspace name is valid, if the given Kubernetes context is valid, if the Radius
// control plane is installed on the target platform, if the workspace already exists, if the user has specified the
// --force flag, if the given resource group and environment exist, and returns an error if any of these checks fail.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspaceName, err := cli.ReadWorkspaceNameSecondArg(cmd, args)
	if err != nil {
		return err
	}

	kubeContextList, err := r.KubernetesInterface.GetKubeContext()
	if err != nil {
		return clierrors.Message("Failed to read Kubernetes configuration. Ensure you have a valid Kubeconfig file and try again.")
	}
	context, err := cli.RequireKubeContext(cmd, kubeContextList.CurrentContext)
	if err != nil {
		return err
	}

	_, ok := kubeContextList.Contexts[context]
	if !ok {
		return fmt.Errorf("the kubeconfig does not contain a context called %q", context)
	}

	if workspaceName == "" {
		workspaceName = context
	}

	state, err := r.HelmInterface.CheckRadiusInstall(context)
	if !state.RadiusInstalled || err != nil {
		return fmt.Errorf("unable to create workspace %q. Radius control plane not installed on target platform. Run 'rad install' and try again", workspaceName)
	}

	workspaceExists, err := cli.HasWorkspace(config, workspaceName)
	if err != nil {
		return err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force && workspaceExists {
		return fmt.Errorf("workspace exists. please specify --force to overwrite")
	}

	if workspaceExists {
		workspace, err := cli.GetWorkspace(config, workspaceName)
		if err != nil {
			return err
		}
		r.Workspace = workspace
	} else {
		r.Workspace = &workspaces.Workspace{}
		r.Workspace.Name = workspaceName
	}
	r.Workspace.Connection = map[string]any{}
	r.Workspace.Connection["context"] = context
	r.Workspace.Connection["kind"] = args[0]

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	env, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	var client clients.ApplicationsManagementClient
	if group != "" {
		r.Workspace.Scope = "/planes/radius/local/resourceGroups/" + group

		client, err = r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
		if err != nil {
			return err
		}
		_, err := client.GetResourceGroup(cmd.Context(), "local", group)
		if err != nil {
			return clierrors.Message("The resource group %q does not exist. Run `rad env create` try again.", r.Workspace.Scope)
		}

		//we want to make sure we dont have a workspace which has environment in a different scope from workspace's scope
		if r.Workspace.Environment != "" && !strings.HasPrefix(r.Workspace.Environment, r.Workspace.Scope) && env == "" {
			return clierrors.Message("The workspace is currently using an environment which is in different scope. Use -e to specify an environment which is in the scope of this workspace.")
		}
	}
	if env != "" {
		if r.Workspace.Scope == "" {
			return clierrors.Message("Cannot set environment for workspace with empty scope. Use --group to set a scope.")
		}

		validator := r.EnvironmentValidator
		if validator == nil {
			validator = ValidateApplicationsCoreEnvironment
		}
		envID, err := validator(cmd.Context(), r.Workspace, client, env)
		if err != nil {
			return err
		}
		r.Workspace.Environment = envID
	}

	return nil
}

// Run runs the `rad workspace create` command.
//

// Run creates a workspace and sets it as the current workspace, returning an error if any occurs during the process."
func (r *Runner) Run(ctx context.Context) error {

	err := r.ConfigFileInterface.EditWorkspaces(ctx, r.ConfigHolder.Config, r.Workspace)
	if err != nil {
		return err
	}
	r.Output.LogInfo("Local workspace %s created (current)", r.Workspace.Name)

	return nil
}
