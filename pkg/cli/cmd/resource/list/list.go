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

package list

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

// NewCommand creates a new Cobra command and a Runner to list resources of a specified type, or all resources
// regardless of type, in an application or the default environment. It adds flags for application name,
// environment name, resource group, output and workspace.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	return newCommand(factory, false)
}

// NewPreviewCommand creates the preview `rad resource list` command.
func NewPreviewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	return newCommand(factory, true)
}

func newCommand(factory framework.Factory, preview bool) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	runner.Preview = preview

	cmd := &cobra.Command{
		Use:   "list [resourceType]",
		Short: "Lists resources",
		Long:  "List all resources of a specified type. If no resource type is given, lists all resources of any type in an environment or application.",
		Example: `
sample list of resourceType: Applications.Core/containers, Applications.Core/gateways, Applications.Dapr/daprPubSubBrokers, Applications.Core/extenders, Applications.Datastores/mongoDatabases, Applications.Messaging/rabbitMQMessageQueues, Applications.Datastores/redisCaches, Applications.Datastores/sqlDatabases, Applications.Dapr/daprStateStores, Applications.Dapr/daprSecretStores

# list all resources of a specified type in the default environment

rad resource list Applications.Core/containers
rad resource list Applications.Core/gateways

# list all resources of a specified type in an application
rad resource list Applications.Core/containers --application icecream-store

# list all resources of a specified type in an application (shorthand flag)
rad resource list Applications.Core/containers -a icecream-store

# list all resources of a specified type in a specified environment
rad resource list Applications.Core/containers -e not-default-env

# list all resources of any type in the default environment
rad resource list

# list all resources of any type in a specified environment
rad resource list -e not-default-env

# list all resources of any type in an application
rad resource list -a icecream-store

# list preview resources in a Radius.Core environment or application
rad resource list -e not-default-env --preview
rad resource list -a icecream-store --preview
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource list` command.
type Runner struct {
	ConfigHolder              *framework.ConfigHolder
	UCPClientFactory          *v20231001preview.ClientFactory
	RadiusCoreClientFactory   *corerpv20250801.ClientFactory
	ConnectionFactory         connections.Factory
	Output                    output.Interface
	Workspace                 *workspaces.Workspace
	ApplicationName           string
	ApplicationID             string
	EnvironmentNameOrID       string
	Format                    string
	Preview                   bool
	ResourceType              string
	ResourceTypeSuffix        string
	ResourceProviderNamespace string
}

// NewRunner creates a new instance of the `rad resource list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource list` command.
//

// Validate checks the command line args, workspace, scope, application name, resource type and output format, and
// returns an error if any of these are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	applicationName, err := cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}
	r.ApplicationName = applicationName

	environmentFlag, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	if r.ApplicationName != "" && environmentFlag != "" {
		return clierrors.Message("The '-e/--environment' flag cannot be combined with '-a/--application'. An application belongs to a single environment.")
	}

	if r.Preview && r.ApplicationName != "" {
		err = r.resolvePreviewApplication()
		if err != nil {
			return err
		}
	}

	if len(args) > 0 {
		r.ResourceProviderNamespace, r.ResourceTypeSuffix, err = cli.RequireFullyQualifiedResourceType(args)
		if err != nil {
			return err
		}
		r.ResourceType = r.ResourceProviderNamespace + "/" + r.ResourceTypeSuffix

		if environmentFlag != "" {
			// A resource type was given along with the environment flag, so scope the typed list to that environment.
			r.EnvironmentNameOrID, err = r.requireEnvironmentNameOrID(cmd, args)
			if err != nil {
				return err
			}
		}
	} else if r.ApplicationName == "" {
		// No resource type or application was given, so list all resources (of any type) in an environment.
		r.EnvironmentNameOrID, err = r.requireEnvironmentNameOrID(cmd, args)
		if err != nil {
			return err
		}
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource list` command.
//

// Run checks if an application name is provided and if so, checks if the application exists in the workspace, then
// lists all resources of the specified type in the application, and finally writes the resources to the output in the
// specified format. If no application name is provided, it lists all resources of the specified type in the
// environment. If no resource type is given, all resources of any type are listed instead. An error is returned if
// the application does not exist in the workspace.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	var resourceList []generated.GenericResource
	if r.ResourceType == "" {
		if r.ApplicationName == "" {
			resourceList, err = client.ListResourcesInEnvironment(ctx, r.EnvironmentNameOrID)
		} else {
			err = r.requireApplicationExists(ctx, client)
			if err == nil {
				resourceList, err = client.ListResourcesInApplication(ctx, r.applicationNameOrID())
			}
		}
	} else {
		// Initialize the client factory if it hasn't been set externally.
		// This allows for flexibility where a test UCPClientFactory can be set externally during testing.
		if r.UCPClientFactory == nil {
			clientFactory, err := cmd.InitializeClientFactory(ctx, r.Workspace)
			if err != nil {
				return err
			}
			r.UCPClientFactory = clientFactory
		}

		_, err = common.GetResourceTypeDetails(ctx, r.ResourceProviderNamespace, r.ResourceTypeSuffix, r.UCPClientFactory)
		if err != nil {
			return err
		}

		switch {
		case r.ApplicationName != "":
			err = r.requireApplicationExists(ctx, client)
			if err == nil {
				resourceList, err = client.ListResourcesOfTypeInApplication(ctx, r.applicationNameOrID(), r.ResourceType)
			}
		case r.EnvironmentNameOrID != "":
			resourceList, err = client.ListResourcesOfTypeInEnvironment(ctx, r.EnvironmentNameOrID, r.ResourceType)
		default:
			resourceList, err = client.ListResourcesOfType(ctx, r.ResourceType)
		}
	}
	if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetGenericResourceTableFormat())
}

// requireApplicationExists returns a user-facing error if r.ApplicationName does not exist in the workspace.
func (r *Runner) requireApplicationExists(ctx context.Context, client clients.ApplicationsManagementClient) error {
	if r.Preview {
		if r.RadiusCoreClientFactory == nil {
			clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace)
			if err != nil {
				return err
			}
			r.RadiusCoreClientFactory = clientFactory
		}

		_, err := r.RadiusCoreClientFactory.NewApplicationsClient().Get(ctx, r.Workspace.Scope, r.ApplicationName, nil)
		if clients.Is404Error(err) {
			return clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", r.ApplicationName, r.Workspace.Name)
		}
		return err
	}

	_, err := client.GetApplication(ctx, r.ApplicationName)
	if clients.Is404Error(err) {
		return clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", r.ApplicationName, r.Workspace.Name)
	}
	return err
}

func (r *Runner) requireEnvironmentNameOrID(cmd *cobra.Command, args []string) (string, error) {
	if !r.Preview {
		return cli.RequireEnvironmentName(cmd, args, *r.Workspace)
	}

	environmentNameOrID, err := cli.RequireEnvironmentNameOrID(cmd, args, *r.Workspace)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(environmentNameOrID, resources.SegmentSeparator) {
		return resourceID(r.Workspace.Scope, datamodel.EnvironmentResourceType_v20250801preview, environmentNameOrID), nil
	}

	environmentID, err := resources.ParseResource(environmentNameOrID)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(environmentID.Type(), datamodel.EnvironmentResourceType_v20250801preview) {
		return "", clierrors.Message("The environment ID %q must reference a %s resource when preview mode is enabled.", environmentNameOrID, datamodel.EnvironmentResourceType_v20250801preview)
	}

	return environmentID.String(), nil
}

func (r *Runner) resolvePreviewApplication() error {
	if !strings.HasPrefix(r.ApplicationName, resources.SegmentSeparator) {
		r.ApplicationID = resourceID(r.Workspace.Scope, datamodel.ApplicationResourceType_v20250801preview, r.ApplicationName)
		return nil
	}

	applicationID, err := resources.ParseResource(r.ApplicationName)
	if err != nil {
		return err
	}
	if !strings.EqualFold(applicationID.Type(), datamodel.ApplicationResourceType_v20250801preview) {
		return clierrors.Message("The application ID %q must reference a %s resource when preview mode is enabled.", r.ApplicationName, datamodel.ApplicationResourceType_v20250801preview)
	}

	r.ApplicationName = applicationID.Name()
	r.ApplicationID = applicationID.String()
	return nil
}

func (r *Runner) applicationNameOrID() string {
	if r.Preview {
		return r.ApplicationID
	}
	return r.ApplicationName
}

func resourceID(scope string, resourceType string, name string) string {
	return scope + "/providers/" + resourceType + "/" + name
}
