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

// Package preview implements the `rad workspace create --preview` command. It reuses
// the runner from the parent create package and only overrides the environment
// validation step so that the workspace is bound to a Radius.Core/environments
// resource (v20250801preview) instead of an Applications.Core/environments resource.
package preview

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	workspace_create "github.com/radius-project/radius/pkg/cli/cmd/workspace/create"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// NewCommand creates an instance of the command and runner for the `rad workspace create --preview` command.
//
// The preview command behaves identically to `rad workspace create`, but binds the workspace
// to a Radius.Core/environments resource (v20250801preview) instead of an
// Applications.Core/environments resource.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	c := &cobra.Command{
		Use:   "create [workspaceType] [workspaceName]",
		Short: "Create a workspace (preview)",
		Long: `Create a workspace bound to a Radius.Core (preview) environment.

Available workspaceTypes: kubernetes

Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file.

Use this command together with environments created via 'rad env create --preview'.`,
		Args: workspace_create.ValidateArgs(),
		Example: `
# Create a workspace bound to a Radius.Core (preview) environment
rad workspace create kubernetes my-workspace --group my-grp --environment my-env --preview

# Create a workspace using the current kubernetes context as the workspace name
rad workspace create kubernetes --preview`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(c)
	commonflags.AddResourceGroupFlag(c)
	commonflags.AddEnvironmentNameFlag(c)
	c.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	c.Flags().StringP("context", "c", "", "the Kubernetes context to use, will use the default if unset")

	return c, runner
}

// Runner is the runner implementation for the `rad workspace create --preview` command.
//
// It embeds the legacy create.Runner and overrides only the environment validation
// step. All other workspace-creation behaviour (kube context, install check, group
// lookup, persistence) is inherited unchanged.
type Runner struct {
	*workspace_create.Runner

	// RadiusCoreClientFactory validates the Radius.Core/environments resource. When nil,
	// it is initialized lazily from the workspace connection. Tests may pre-populate it.
	RadiusCoreClientFactory *corerpv20250801.ClientFactory
}

// NewRunner creates a new instance of the `rad workspace create --preview` runner.
func NewRunner(factory framework.Factory) *Runner {
	base := workspace_create.NewRunner(factory)
	r := &Runner{Runner: base}
	base.EnvironmentValidator = r.validateRadiusCoreEnvironment
	return r
}

// validateRadiusCoreEnvironment validates that the Radius.Core/environments resource
// exists in the workspace scope and returns its fully-qualified resource ID.
func (r *Runner) validateRadiusCoreEnvironment(ctx context.Context, ws *workspaces.Workspace, _ clients.ApplicationsManagementClient, envName string) (string, error) {
	envID := ws.Scope + "/providers/" + datamodel.EnvironmentResourceType_v20250801preview + "/" + envName

	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, ws, ws.Scope)
		if err != nil {
			return "", err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	if _, err := r.RadiusCoreClientFactory.NewEnvironmentsClient().Get(ctx, envName, nil); err != nil {
		if clients.Is404Error(err) {
			return "", clierrors.Message("The environment %q does not exist. Run `rad env create --preview` and try again.", envID)
		}
		return "", clierrors.MessageWithCause(err, "Failed to get environment %q.", envID)
	}
	return envID, nil
}
