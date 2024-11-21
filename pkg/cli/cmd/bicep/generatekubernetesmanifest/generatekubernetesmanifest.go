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
	"strings"

	"github.com/spf13/afero"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/spf13/cobra"
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
	order they are provided. Parameters appearing later in the argument list will override those defined earlier.
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
	commonflags.AddParameterFlag(cmd)

	cmd.Flags().String("outfile", "", "Path of the generated DeploymentTemplate yaml file.")
	_ = cmd.MarkFlagFilename("outfile", ".yaml")

	cmd.Flags().String("azure-scope", "", "Scope for Azure deployment.")
	cmd.Flags().String("aws-scope", "", "Scope for AWS deployment.")
	cmd.Flags().String("deployment-scope", "", "Scope for the Radius deployment.")

	return cmd, runner
}

// Runner is the runner implementation for the `rad bicep generate-kubernetes` command.
type Runner struct {
	Bicep             bicep.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Deploy            deploy.Interface
	Output            output.Interface

	FileSystem      afero.Fs
	FilePath        string
	Parameters      map[string]map[string]any
	OutFile         string
	DeploymentScope string
	AzureScope      string
	AWSScope        string
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
	r.FilePath = args[0]

	var err error
	r.DeploymentScope, err = cmd.Flags().GetString("deployment-scope")
	if err != nil {
		return err
	}

	if r.DeploymentScope == "" {
		r.DeploymentScope = "/planes/radius/local/resourceGroups/default"
	}

	r.AzureScope, err = cmd.Flags().GetString("azure-scope")
	if err != nil {
		return err
	}

	r.AWSScope, err = cmd.Flags().GetString("aws-scope")
	if err != nil {
		return err
	}

	r.OutFile, err = cmd.Flags().GetString("outfile")
	if err != nil {
		return err
	}

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

	// create a DeploymentTemplate yaml file
	// with the basefilename from the bicepfile
	if r.OutFile == "" {
		r.OutFile = strings.TrimSuffix(filepath.Base(r.FilePath), filepath.Ext(r.FilePath)) + ".yaml"
	}

	deploymentTemplate, err := r.generateDeploymentTemplate(filepath.Base(r.FilePath), template, r.Parameters)
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

// generateDeploymentTemplate generates a DeploymentTemplate Custom Resource from the given template and parameters.
func (r *Runner) generateDeploymentTemplate(fileName string, template map[string]any, parameters map[string]map[string]any) (map[string]any, error) {
	marshalledTemplate, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return nil, err
	}

	providerConfig := r.generateProviderConfig()

	marshalledProviderConfig, err := json.MarshalIndent(providerConfig, "", "  ")
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	for k, v := range parameters {
		params[k] = v["value"].(string)
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
			"parameters":     params,
			"providerConfig": string(marshalledProviderConfig),
			"rootFileName":   fileName,
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

// generateProviderConfig generates a ProviderConfig object based on the given scopes.
func (r *Runner) generateProviderConfig() (providerConfig sdkclients.ProviderConfig) {
	providerConfig = sdkclients.ProviderConfig{}
	if r.AWSScope != "" {
		providerConfig.AWS = &sdkclients.AWS{
			Type: "aws",
			Value: sdkclients.Value{
				Scope: r.AWSScope,
			},
		}
	}
	if r.AzureScope != "" {
		providerConfig.Az = &sdkclients.Az{
			Type: "azure",
			Value: sdkclients.Value{
				Scope: r.AzureScope,
			},
		}
	}
	if r.DeploymentScope != "" {
		providerConfig.Radius = &sdkclients.Radius{
			Type: "radius",
			Value: sdkclients.Value{
				Scope: r.DeploymentScope,
			},
		}
		providerConfig.Deployments = &sdkclients.Deployments{
			Type: "Microsoft.Resources",
			Value: sdkclients.Value{
				Scope: r.DeploymentScope,
			},
		}
	}

	return providerConfig
}
