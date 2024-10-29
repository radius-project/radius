/*
Copyright 2024 The Radius Authors.

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

package bicep

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v2"
)

// NewCommand creates a command for the `rad bicep generate-kubernetes-manifest` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "generate-kubernetes-manifest [file]",
		Short: "Generate a DeploymentTemplate Custom Resource.",
		Long: `Generate a DeploymentTemplate Custom Resource.

	This command compiles a Bicep template with the given parameters and outputs a DeploymentTemplate Custom Resource.

	You can specify parameters using the '--parameter' flag ('-p' for short). Parameters can be passed as:
	
	- A file containing multiple parameters using the ARM JSON parameter format (see below)
	- A file containing a single value in JSON format
	- A key-value-pair passed in the command line
	
	When passing multiple parameters in a single file, use the format described here:
	
		https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files
	
	You can specify parameters using multiple sources. Parameters can be overridden based on the 
	order the are provided. Parameters appearing later in the argument list will override those defined earlier.
		`,
		Example: `
# Generate a DeploymentTemplate Custom Resource from a Bicep file.
rad bicep generate-kubernetes-manifest app.bicep --parameters @app.bicepparam --parameters tag=latest --outfile app.yaml
		`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddParameterFlag(cmd)

	cmd.Flags().String("outfile", "", "Path of the generated DeploymentTemplate yaml file.")
	_ = cmd.MarkFlagFilename("outfile", ".yaml")

	return cmd, runner
}

// Runner is the runner implementation for the `rad bicep generate-kubernetes` command.
type Runner struct {
	Bicep             bicep.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Deploy            deploy.Interface
	Output            output.Interface

	FileSystem          afero.Fs
	EnvironmentNameOrID string
	FilePath            string
	Parameters          map[string]map[string]any
	Workspace           *workspaces.Workspace
	Providers           *clients.Providers
	OutFile             string
}

// NewRunner creates a new instance of the `rad deploy` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Bicep:             factory.GetBicep(),
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Deploy:            factory.GetDeploy(),
		Output:            factory.GetOutput(),
	}
}

// Validate validates the inputs of the rad bicep generate-kubernetes-manifest command.
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

	r.EnvironmentNameOrID, err = cli.RequireEnvironmentNameOrID(cmd, args, *workspace)
	if err != nil {
		return err
	}

	// Validate that the environment exists.
	// Right now we assume that every deployment uses a Radius Environment.
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}
	env, err := client.GetEnvironment(cmd.Context(), r.EnvironmentNameOrID)
	if err != nil {
		// If the error is not a 404, return it
		if !clients.Is404Error(err) {
			return err
		}

		// If the environment doesn't exist, but the user specified its name or resource id as
		// a command-line option, return an error
		if cli.DidSpecifyEnvironmentName(cmd, args) {
			return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.", r.EnvironmentNameOrID, r.Workspace.Scope)
		}

		// If we got here, it means that the error was a 404 and the user did not specify the environment name.
		// This is fine, because an environment is not required.
	}

	r.Providers = &clients.Providers{}
	r.Providers.Radius = &clients.RadiusProvider{}
	if env.ID != nil {
		r.Providers.Radius.EnvironmentID = *env.ID
		r.Workspace.Environment = r.Providers.Radius.EnvironmentID
	}

	if env.Properties != nil && env.Properties.Providers != nil {
		if env.Properties.Providers.Aws != nil {
			r.Providers.AWS = &clients.AWSProvider{
				Scope: *env.Properties.Providers.Aws.Scope,
			}
		}
		if env.Properties.Providers.Azure != nil {
			r.Providers.Azure = &clients.AzureProvider{
				Scope: *env.Properties.Providers.Azure.Scope,
			}
		}
	}

	r.FilePath = args[0]

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	if r.FileSystem == nil {
		r.FileSystem = afero.NewOsFs()
	}

	parser := bicep.ParameterParser{FileSystem: r.FileSystem}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the rad bicep generate-kubernetes-manifest command.
func (r *Runner) Run(ctx context.Context) error {
	template, err := r.Bicep.PrepareTemplate(r.FilePath)
	if err != nil {
		return err
	}

	// This is the earliest point where we can inject parameters, we have
	// to wait until the template is prepared.
	err = r.injectAutomaticParameters(template)
	if err != nil {
		return err
	}

	// This is the earliest point where we can report missing parameters, we have
	// to wait until the template is prepared.
	err = r.reportMissingParameters(template)
	if err != nil {
		return err
	}

	// create a DeploymentTemplate yaml file
	// with the basefilename from the bicepfile
	if r.OutFile == "" {
		r.OutFile = strings.TrimSuffix(filepath.Base(r.FilePath), filepath.Ext(r.FilePath)) + ".yaml"
	}

	deploymentTemplate, err := r.generateDeploymentTemplate(r.OutFile, template, r.Parameters, r.Providers)
	if err != nil {
		return err
	}

	err = r.createDeploymentTemplateYAMLFile(deploymentTemplate)
	if err != nil {
		return err
	}

	// Print the path to the file
	r.Output.LogInfo("DeploymentTemplate file created at %s", r.OutFile)

	return nil
}

func (r *Runner) injectAutomaticParameters(template map[string]any) error {
	if r.Providers.Radius.EnvironmentID != "" {
		err := bicep.InjectEnvironmentParam(template, r.Parameters, r.Providers.Radius.EnvironmentID)
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

// generateDeploymentTemplate generates a DeploymentTemplate Custom Resource from the given template and parameters.
func (r *Runner) generateDeploymentTemplate(fileName string, template map[string]any, parameters map[string]map[string]any, providers *clients.Providers) (map[string]any, error) {
	marshalledTemplate, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}

	marshalledParameters, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}

	providerConfig := r.convertProvidersToProviderConfig(providers)

	marshalledProviderConfig, err := json.Marshal(providerConfig)
	if err != nil {
		return nil, err
	}

	deploymentTemplate := map[string]any{
		"kind":       "DeploymentTemplate",
		"apiVersion": "radapp.io/v1alpha3",
		"metadata": map[string]any{
			"name":      fileName,
			"namespace": "radius-system",
		},
		"spec": map[string]any{
			"template":       string(marshalledTemplate),
			"parameters":     string(marshalledParameters),
			"providerConfig": string(marshalledProviderConfig),
		},
	}

	return deploymentTemplate, nil
}

// createDeploymentTemplateYAMLFile creates a DeploymentTemplate YAML file with the given content.
func (r *Runner) createDeploymentTemplateYAMLFile(deploymentTemplate map[string]any) error {
	fmt.Println("Creating DeploymentTemplate YAML file")
	f, err := r.FileSystem.Create(r.OutFile)
	if err != nil {
		return err
	}

	defer f.Close()

	deploymentTemplateYaml, err := yaml.Marshal(deploymentTemplate)
	if err != nil {
		return err
	}

	_, err = f.Write(deploymentTemplateYaml)
	if err != nil {
		return err
	}

	return nil
}

// convertProvidersToProviderConfig converts the the clients.Providers to sdkclients.ProviderConfig.
func (r *Runner) convertProvidersToProviderConfig(providers *clients.Providers) (providerConfig sdkclients.ProviderConfig) {
	providerConfig = sdkclients.ProviderConfig{}
	if providers != nil {
		if providers.AWS != nil {
			providerConfig.AWS = &sdkclients.AWS{
				Type: "aws",
				Value: sdkclients.Value{
					Scope: providers.AWS.Scope,
				},
			}
		}
		if providers.Azure != nil {
			providerConfig.Az = &sdkclients.Az{
				Type: "azure",
				Value: sdkclients.Value{
					Scope: providers.Azure.Scope,
				},
			}
		}
		if providers.Radius != nil {
			providerConfig.Radius = &sdkclients.Radius{
				Type: "radius",
				Value: sdkclients.Value{
					Scope: r.Workspace.Scope,
				},
			}
			providerConfig.Deployments = &sdkclients.Deployments{
				Type: "Microsoft.Resources",
				Value: sdkclients.Value{
					Scope: r.Workspace.Scope,
				},
			}
		}
	}

	return providerConfig
}
