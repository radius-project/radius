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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

const (
	resourceGroupRequiredMessage = "Radius resource group is required. Please provide a value for the --group (-g) flag."
	sourceRevisionAnnotation     = "radapp.io/source-revision"
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
	
	- A file containing multiple parameters using the ARM JSON parameter format (see below) or a Bicep parameters (.bicepparam) file
	- A file containing a single value in JSON format
	- A key-value-pair passed in the command line
	
	When passing multiple parameters in a single file, use the format described here:
	
		https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files
	
	You can specify parameters using multiple sources. Parameters can be overridden based on the 
	order they are provided. Parameters appearing later in the argument list will override those defined earlier.
		`,
		Example: `
# Generate a DeploymentTemplate Custom Resource from a Bicep file.
rad bicep generate-kubernetes-manifest app.bicep --parameters @app.bicepparam --parameters tag=latest --destination-file app.yaml --resource-group default
		`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddParameterFlag(cmd)

	cmd.Flags().StringP("destination-file", "d", "", "Path of the generated DeploymentTemplate yaml file created by running this command, or '-' to write to stdout.")
	_ = cmd.MarkFlagFilename("destination-file", ".yaml", ".yml")

	cmd.Flags().String("azure-scope", "", "Scope for Azure deployment.")
	cmd.Flags().String("aws-scope", "", "Scope for AWS deployment.")
	cmd.Flags().String("name", "", "Name of the generated DeploymentTemplate resource. Defaults to the template file name.")
	cmd.Flags().String("source-revision", "", "Source revision to record in the radapp.io/source-revision annotation.")

	return cmd, runner
}

// Runner is the runner implementation for the `rad bicep generate-kubernetes` command.
type Runner struct {
	Bicep             bicep.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Deploy            deploy.Interface
	Output            output.Interface
	Stdout            io.Writer

	FileSystem      filesystem.FileSystem
	Group           string
	FilePath        string
	Parameters      map[string]map[string]any
	DestinationFile string
	AzureScope      string
	AWSScope        string
	Name            string
	SourceRevision  string
}

// NewRunner creates a new instance of the `rad bicep generate-kubernetes-manifest` runner.
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
	r.Group, err = cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	if r.Group == "" {
		return clierrors.Message(resourceGroupRequiredMessage)
	}

	r.AzureScope, err = cmd.Flags().GetString("azure-scope")
	if err != nil {
		return err
	}

	r.AWSScope, err = cmd.Flags().GetString("aws-scope")
	if err != nil {
		return err
	}

	r.DestinationFile, err = cmd.Flags().GetString("destination-file")
	if err != nil {
		return err
	}

	// If the destination file is not provided, use the base name of the file with a .yaml extension.
	// The name is derived from the template file name only, so a credential in a remote URL's query
	// string is never used to build an on-disk file name.
	if r.DestinationFile == "" {
		templateFileName := bicep.TemplateFileName(r.FilePath)
		r.DestinationFile = strings.TrimSuffix(templateFileName, filepath.Ext(templateFileName)) + ".yaml"
	}

	if r.DestinationFile != "-" && filepath.Ext(r.DestinationFile) != ".yaml" && filepath.Ext(r.DestinationFile) != ".yml" {
		return clierrors.Message("Destination file must have a .yaml or .yml extension")
	}

	if r.Stdout == nil {
		r.Stdout = cmd.OutOrStdout()
	}

	if r.DestinationFile == "-" {
		if ow, ok := r.Output.(*output.OutputWriter); ok && ow.LogWriter == nil {
			ow.LogWriter = cmd.ErrOrStderr()
		}
	}

	r.Name, err = cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	r.SourceRevision, err = cmd.Flags().GetString("source-revision")
	if err != nil {
		return err
	}

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	if r.FileSystem == nil {
		r.FileSystem = filesystem.NewOSFS()
	}

	parser := bicep.ParameterParser{
		FileSystem: r.FileSystem,
		Bicep:      r.Bicep,
	}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the rad bicep generate-kubernetes-manifest command.
func (r *Runner) Run(ctx context.Context) error {
	template, _, err := r.Bicep.PrepareTemplate(ctx, r.FilePath)
	if err != nil {
		return err
	}

	name := r.Name
	if name == "" {
		// The template name is written into the generated manifest, so derive it from the template file
		// name only to keep credentials in a remote URL out of the generated output.
		name = bicep.TemplateFileName(r.FilePath)
	}

	deploymentTemplate, err := r.generateDeploymentTemplate(name, template, r.Parameters)
	if err != nil {
		return err
	}

	err = r.createDeploymentTemplateYAMLFile(deploymentTemplate)
	if err != nil {
		return err
	}

	if r.DestinationFile != "-" {
		r.Output.LogInfo("DeploymentTemplate file created at %s", r.DestinationFile)
	}

	return nil
}

// generateDeploymentTemplate generates a DeploymentTemplate Custom Resource from the given template and parameters.
func (r *Runner) generateDeploymentTemplate(fileName string, template map[string]any, parameters map[string]map[string]any) (map[string]any, error) {
	marshalledTemplate, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return nil, err
	}

	providerConfig, err := sdkclients.GenerateProviderConfig(r.Group, r.AWSScope, r.AzureScope).String()
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	for k, v := range parameters {
		val, ok := v["value"]
		if !ok || val == nil {
			continue
		}
		if strVal, ok := val.(string); ok {
			params[k] = strVal
			continue
		}
		marshalledVal, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		params[k] = string(marshalledVal)
	}

	metadata := map[string]any{
		"name": fileName,
	}
	if r.SourceRevision != "" {
		metadata["annotations"] = map[string]string{
			sourceRevisionAnnotation: r.SourceRevision,
		}
	}

	spec := map[string]any{
		"template":       string(marshalledTemplate),
		"providerConfig": providerConfig,
	}
	if len(params) > 0 {
		spec["parameters"] = params
	}

	deploymentTemplate := map[string]any{
		"kind":       "DeploymentTemplate",
		"apiVersion": "radapp.io/v1alpha3",
		"metadata":   metadata,
		"spec":       spec,
	}

	return deploymentTemplate, nil
}

// createDeploymentTemplateYAMLFile creates a DeploymentTemplate YAML file with the given content,
// or writes it to stdout when DestinationFile is "-".
func (r *Runner) createDeploymentTemplateYAMLFile(deploymentTemplate map[string]any) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	encoder.SetIndent(2)

	err := encoder.Encode(deploymentTemplate)
	if err != nil {
		return err
	}

	if r.DestinationFile == "-" {
		out := r.Stdout
		if out == nil {
			out = os.Stdout
		}
		_, err = out.Write(buf.Bytes())
		return err
	}

	return r.FileSystem.WriteFile(r.DestinationFile, buf.Bytes(), 0644)
}
