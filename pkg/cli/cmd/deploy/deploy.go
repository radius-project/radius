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
	"sort"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
)

const (
	appCoreProviderName    = "Applications.Core"
	radiusCoreProviderName = "Radius.Core"
)

// NewCommand creates an instance of the command and runner for the `rad deploy` command.
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
order they are provided. Parameters appearing later in the argument list will override those defined earlier.
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

# deploy using an environment ID and a resource group. The application will be deployed in mygroup scope, using the specified environment.
# use this option if the environment is in a different group.
rad deploy myapp.bicep --environment /planes/radius/local/resourcegroups/prod/providers/Applications.Core/environments/prod --group mygroup

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
	Bicep                   bicep.Interface
	ConfigHolder            *framework.ConfigHolder
	ConnectionFactory       connections.Factory
	RadiusCoreClientFactory *v20250801preview.ClientFactory
	Deploy                  deploy.Interface
	Output                  output.Interface

	ApplicationName     string
	EnvironmentNameOrID string
	FilePath            string
	Parameters          map[string]map[string]any
	Template            map[string]any
	Workspace           *workspaces.Workspace
	Providers           *clients.Providers
	EnvResult           *EnvironmentCheckResult
}

// NewRunner creates a new instance of the `rad deploy` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Bicep:             factory.GetBicep(),
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Deploy:            factory.GetDeploy(),
		Output:            factory.GetOutput(),
		Providers:         &clients.Providers{},
	}
}

// Validate runs validation for the `rad deploy` command.
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

	// Get the file path early so we can prepare the template
	r.FilePath = args[0]

	// Prepare the template early to check if it contains an environment resource.
	// This allows us to skip environment validation if the template will create one.
	r.Template, err = r.Bicep.PrepareTemplate(r.FilePath)
	if err != nil {
		return err
	}

	// Check if environment was explicitly provided via flag or workspace default
	environmentFlag, _ := cmd.Flags().GetString("environment")
	environmentProvidedExplicitly := environmentFlag != "" || workspace.Environment != ""

	// Check if the template contains an environment resource
	templateCreatesEnvironment := bicep.ContainsEnvironmentResource(r.Template)

	if !templateCreatesEnvironment || environmentProvidedExplicitly {
		// Environment is required if:
		// 1. Template doesn't create environment, OR
		// 2. User explicitly provided --environment flag or workspace has default environment
		r.EnvironmentNameOrID, err = cli.RequireEnvironmentNameOrID(cmd, args, *workspace)
		if err != nil {
			return err
		}
	} else {
		// Template creates the environment and no environment was explicitly provided
		// Set to empty string to indicate no pre-existing environment
		r.EnvironmentNameOrID = ""
	}

	// This might be empty, and that's fine!
	r.ApplicationName, err = cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}

	if r.EnvironmentNameOrID != "" {
		envResult, err := r.FetchEnvironment(cmd.Context(), r.EnvironmentNameOrID)
		if err != nil {
			return err
		}
		if envResult == nil {
			return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.", r.EnvironmentNameOrID, r.Workspace.Scope)
		}
		r.EnvResult = envResult
	}

	err = r.configureProviders()
	if err != nil {
		return err
	}

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: filesystem.NewOSFS()}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad deploy` command.
//

// Run deploys a Bicep template into an environment from a workspace, optionally creating an application if
// specified, and displays progress and completion messages. It returns an error if any of the operations fail.
func (r *Runner) Run(ctx context.Context) error {
	// Use the template that was prepared during validation
	template := r.Template

	// This is the earliest point where we can inject parameters, we have
	// to wait until the template is prepared.
	err := r.injectAutomaticParameters(template)
	if err != nil {
		return err
	}

	// This is the earliest point where we can report missing parameters, we have
	// to wait until the template is prepared.
	err = r.reportMissingParameters(template)
	if err != nil {
		return err
	}

	// Create application if specified. This supports the case where the application resource
	// is not specified in Bicep. Creating the application automatically helps us "bootstrap" in a new environment.
	// Note: This only applies when the environment already exists. If the template is creating the environment,
	// r.EnvironmentNameOrID will be empty and we'll skip this step (the template deployment will create
	// whatever resources it defines).

	if r.ApplicationName != "" {
		// Environment validation has already happened, so only create application if we have an environment
		if r.Providers.Radius.EnvironmentID != "" {
			if _, err := isApplicationsCoreProvider(r.Providers.Radius.EnvironmentID); err == nil {
				client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
				if err != nil {
					return err
				}
				err = client.CreateApplicationIfNotFound(ctx, r.ApplicationName, &v20231001preview.ApplicationResource{
					Location: to.Ptr(v1.LocationGlobal),
					Properties: &v20231001preview.ApplicationProperties{
						Environment: &r.Providers.Radius.EnvironmentID,
					},
				})
				if err != nil {
					return err
				}
			} else {
				client := r.RadiusCoreClientFactory.NewApplicationsClient()
				_, err := client.Get(ctx, r.ApplicationName, nil)
				if err != nil {
					if clients.Is404Error(err) {
						_, err = client.CreateOrUpdate(ctx, r.ApplicationName, v20250801preview.ApplicationResource{
							Location: to.Ptr(v1.LocationGlobal),
							Properties: &v20250801preview.ApplicationProperties{
								Environment: &r.Providers.Radius.EnvironmentID,
							},
						}, nil)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				}
			}
		}
	}

	progressText := ""
	if r.ApplicationName == "" {
		progressText = fmt.Sprintf(
			"Deploying template '%v' into environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress...", r.FilePath, r.EnvironmentNameOrID, r.Workspace.Name)
	} else {
		progressText = fmt.Sprintf(
			"Deploying template '%v' for application '%v' and environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress... ", r.FilePath, r.ApplicationName, r.EnvironmentNameOrID, r.Workspace.Name)
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

func (r *Runner) injectAutomaticParameters(template map[string]any) error {
	if r.Providers.Radius.EnvironmentID != "" {
		err := bicep.InjectEnvironmentParam(template, r.Parameters, r.Providers.Radius.EnvironmentID)
		if err != nil {
			return err
		}
	}

	if r.Providers.Radius.ApplicationID != "" {
		err := bicep.InjectApplicationParam(template, r.Parameters, r.Providers.Radius.ApplicationID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) reportMissingParameters(template map[string]any) error {
	declaredParameters, err := bicep.ExtractParameters(template)
	if err != nil {
		return err
	}

	errors := map[string]string{}
	for parameter := range declaredParameters {
		// Case-invariant lookup on the user-provided values
		match := false
		for provided := range r.Parameters {
			if strings.EqualFold(parameter, provided) {
				match = true
				break
			}
		}

		if match {
			// Has user-provided value
			continue
		}

		if _, ok := bicep.DefaultValue(declaredParameters[parameter]); ok {
			// Has default value
			continue
		}

		// Special case the parameters that are automatically injected
		if strings.EqualFold(parameter, "environment") {
			errors[parameter] = "The template requires an environment. Use --environment to specify the environment name."
		} else if strings.EqualFold(parameter, "application") {
			errors[parameter] = "The template requires an application. Use --application to specify the application name."
		} else {
			errors[parameter] = fmt.Sprintf("The template requires a parameter %q. Use --parameters %s=<value> to specify the value.", parameter, parameter)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	keys := maps.Keys(errors)
	sort.Strings(keys)

	details := []string{}
	for _, key := range keys {
		details = append(details, fmt.Sprintf("  - %v", errors[key]))
	}

	return clierrors.Message("The template %q could not be deployed because of the following errors:\n\n%v", r.FilePath, strings.Join(details, "\n"))
}

// isApplicationsCoreProvider returns true if the provider is Applications.Core based on the environment ID
// It returns an error if the ID cannot be parsed
func isApplicationsCoreProvider(id string) (bool, error) {
	parsedID, err := resources.Parse(id)
	if err != nil {
		return false, err
	}

	providerNamespace := parsedID.ProviderNamespace()
	if strings.EqualFold(providerNamespace, appCoreProviderName) {
		return true, nil
	}
	return false, nil
}

// handleEnvironmentError handles common error patterns for environment retrieval
func (r *Runner) handleEnvironmentError(err error, command *cobra.Command, args []string) error {
	// If the error is not a 404, return it
	if !clients.Is404Error(err) {
		return err
	}

	// If the environment doesn't exist, but the user specified its name or resource id as
	// a command-line option, return an error
	if r.EnvironmentNameOrID != "" {
		// Extract environment name from ID for better error message
		envName := r.EnvironmentNameOrID
		if parsedID, err := resources.Parse(r.EnvironmentNameOrID); err == nil {
			envName = parsedID.Name()
		}
		return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.", envName, r.Workspace.Scope)
	}

	// If we got here, it means that the error was a 404 and no environment was specified anywhere.
	// This is fine, because an environment is not required.
	return nil
}

// setupEnvironmentID sets up the environment ID and workspace environment
func (r *Runner) setupEnvironmentID(envID *string) {
	if envID != nil && r.Providers != nil && r.Providers.Radius != nil {
		r.Providers.Radius.EnvironmentID = *envID
		r.Workspace.Environment = r.Providers.Radius.EnvironmentID
	}
}

// getApplicationsCoreEnvironment retrieves environment using Applications Core client
func (r *Runner) getApplicationsCoreEnvironment(ctx context.Context, id string) (*v20231001preview.EnvironmentResource, error) {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return nil, err
	}
	env, err := client.GetEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

// getRadiusCoreEnvironment retrieves environment using Radius Core client and returns as Applications.Core format
func (r *Runner) getRadiusCoreEnvironment(ctx context.Context, id string) (*v20250801preview.EnvironmentResource, error) {
	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return nil, err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	environmentClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()
	env, err := environmentClient.Get(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return &env.EnvironmentResource, nil
}

// constructEnvironmentID constructs an environment ID from a name and provider type
func (r *Runner) constructEnvironmentID(envName, providerType string) string {
	return r.Workspace.Scope + "/providers/" + providerType + "/environments/" + envName
}

// constructApplicationsCoreEnvironmentID constructs an Applications.Core environment ID from a name
func (r *Runner) constructApplicationsCoreEnvironmentID(envNameOrID string) string {
	return r.constructEnvironmentID(envNameOrID, appCoreProviderName)
}

// constructRadiusCoreEnvironmentID constructs a Radius.Core environment ID from a name
func (r *Runner) constructRadiusCoreEnvironmentID(envName string) string {
	return r.constructEnvironmentID(envName, radiusCoreProviderName)
}

// EnvironmentCheckResult holds the result of checking for environments
type EnvironmentCheckResult struct {
	UseApplicationsCore bool
	ApplicationsCoreEnv *v20231001preview.EnvironmentResource
	RadiusCoreEnv       *v20250801preview.EnvironmentResource
}

// FetchEnvironment fetches Applications.Core and Radius.Core environments for a given name/id and returns the result
// If no environment is found, returns (nil, nil)
func (r *Runner) FetchEnvironment(ctx context.Context, envNameOrID string) (*EnvironmentCheckResult, error) {
	result := &EnvironmentCheckResult{}
	// If the environment is specified as a full resource ID, we can skip the check and based on the provider get the environment
	fetchAppCoreEnv := true
	fetchRadiusCoreEnv := true

	envID, err := resources.Parse(envNameOrID)
	isID := false
	if err == nil {
		isID = true
		if strings.EqualFold(envID.ProviderNamespace(), appCoreProviderName) {
			fetchRadiusCoreEnv = false
		} else {
			fetchAppCoreEnv = false
		}
	}

	// Check Applications.Core environment
	if fetchAppCoreEnv {
		// If its ID, use it directly, otherwise construct ID from name
		var appCoreEnvID string
		if !isID {
			appCoreEnvID = r.constructApplicationsCoreEnvironmentID(envNameOrID)
		} else {
			appCoreEnvID = envNameOrID
		}
		appCoreEnv, err := r.getApplicationsCoreEnvironment(ctx, appCoreEnvID)
		if err != nil {
			if !clients.Is404Error(err) {
				return nil, err
			}
		}
		if appCoreEnv != nil {
			result.ApplicationsCoreEnv = appCoreEnv
		}
	}
	if fetchRadiusCoreEnv {
		var radCoreEnvID string
		if !isID {
			radCoreEnvID = r.constructRadiusCoreEnvironmentID(envNameOrID)
		} else {
			radCoreEnvID = envNameOrID
		}

		radiusCoreEnv, err := r.getRadiusCoreEnvironment(ctx, radCoreEnvID)
		if err != nil {
			if !clients.Is404Error(err) {
				return nil, err
			}
		}
		if radiusCoreEnv != nil {
			result.RadiusCoreEnv = radiusCoreEnv
		}
	}

	// Determine which one to use and check for conflicts
	if result.ApplicationsCoreEnv != nil && result.RadiusCoreEnv != nil {
		var appCoreID, radiusCoreID string
		if result.ApplicationsCoreEnv.ID != nil {
			appCoreID = *result.ApplicationsCoreEnv.ID
		}
		if result.RadiusCoreEnv.ID != nil {
			radiusCoreID = *result.RadiusCoreEnv.ID
		}
		return nil, clierrors.Message("Conflict detected: Environment '%s' exists in both Applications.Core and Radius.Core providers. Please specify the full resource ID to disambiguate:\n  Applications.Core: %s\n  Radius.Core: %s",
			envNameOrID, appCoreID, radiusCoreID)
	}

	if result.ApplicationsCoreEnv != nil {
		result.UseApplicationsCore = true
		if result.ApplicationsCoreEnv.ID != nil {
			r.EnvironmentNameOrID = *result.ApplicationsCoreEnv.ID
		}
	} else if result.RadiusCoreEnv != nil {
		result.UseApplicationsCore = false
		if result.RadiusCoreEnv.ID != nil {
			r.EnvironmentNameOrID = *result.RadiusCoreEnv.ID
		}
	} else {
		// Neither found, treat as environment not found case
		return nil, nil
	}

	return result, nil
}

// setupCloudProviders sets up AWS and Azure providers based on environment properties
func (r *Runner) setupCloudProviders(properties any) {
	switch props := properties.(type) {
	case *v20231001preview.EnvironmentProperties:
		if props != nil && props.Providers != nil {
			if props.Providers.Aws != nil {
				r.Providers.AWS = &clients.AWSProvider{
					Scope: *props.Providers.Aws.Scope,
				}
			}
			if props.Providers.Azure != nil {
				r.Providers.Azure = &clients.AzureProvider{
					Scope: *props.Providers.Azure.Scope,
				}
			}
		}
	case *v20250801preview.EnvironmentProperties:
		if props != nil && props.Providers != nil {
			if props.Providers.Aws != nil {
				r.Providers.AWS = &clients.AWSProvider{
					Scope: *props.Providers.Aws.Scope,
				}
			}
			if props.Providers.Azure != nil {
				r.Providers.Azure = &clients.AzureProvider{
					Scope: "/planes/azure/azure/" + "Subscriptions/" + *props.Providers.Azure.SubscriptionID + "/ResourceGroups/" + *props.Providers.Azure.ResourceGroupName,
				}
			}
		}
	}
}

// configureProviders configures environment and cloud providers based on the environment and provider type
func (r *Runner) configureProviders() error {
	var env any
	if r.Providers == nil {
		r.Providers = &clients.Providers{}
	}
	if r.Providers.Radius == nil {
		r.Providers.Radius = &clients.RadiusProvider{}
	}

	if r.EnvResult != nil {
		if r.EnvResult.UseApplicationsCore {
			if r.EnvResult.ApplicationsCoreEnv != nil {
				env = r.EnvResult.ApplicationsCoreEnv
			}
		} else {
			if r.EnvResult.RadiusCoreEnv != nil {
				env = r.EnvResult.RadiusCoreEnv
			}
		}
	} else {
		return nil
	}

	switch e := env.(type) {
	case *v20231001preview.EnvironmentResource:
		if e != nil && e.ID != nil {
			r.setupEnvironmentID(e.ID)
			r.setupCloudProviders(e.Properties)
		}
		if r.ApplicationName != "" {
			// Extract provider namespace from environment ID to preserve casing
			providerNamespace := appCoreProviderName
			if parsedID, err := resources.Parse(r.Providers.Radius.EnvironmentID); err == nil {
				providerNamespace = parsedID.ProviderNamespace()
			}
			r.Providers.Radius.ApplicationID = r.Workspace.Scope + "/providers/" + providerNamespace + "/applications/" + r.ApplicationName

		}
	case *v20250801preview.EnvironmentResource:
		if e != nil && e.ID != nil {
			r.setupEnvironmentID(e.ID)
			r.setupCloudProviders(e.Properties)
		}
		if r.ApplicationName != "" {
			// Extract provider namespace from environment ID to preserve casing
			providerNamespace := radiusCoreProviderName
			if parsedID, err := resources.Parse(r.Providers.Radius.EnvironmentID); err == nil {
				providerNamespace = parsedID.ProviderNamespace()
			}
			r.Providers.Radius.ApplicationID = r.Workspace.Scope + "/providers/" + providerNamespace + "/applications/" + r.ApplicationName
		}
	}

	return nil
}
