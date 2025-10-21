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

package learn

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	learnLongDescription = `Learn and generate resource type definitions from Terraform modules.

This command analyzes a Terraform module from a Git repository and generates a Radius resource type definition YAML file. The generated definition includes:

- Resource type schema based on Terraform input variables
- Property types converted from Terraform types to JSON schema types  
- Required properties based on variables without default values
- Standard Radius application and environment properties
- Auto-inferred namespace based on module name and provider patterns (e.g., AWS.Network, Azure.Storage)

The generated YAML can be used with 'rad resource-type create' to register the new resource type.
`

	learnExample = `
# Generate resource type from a Terraform module (namespace auto-inferred)
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc

# Override auto-inferred namespace and type name
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc --namespace "MyCompany.AWS" --type-name "vpc"

# Generate and save to specific file
rad resource-type learn --git-url https://github.com/example/terraform-aws-vpc --output vpc-resource-type.yaml
`
)

// NewCommand creates an instance of the `rad resource-type learn` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "learn",
		Short:   "Generate resource type definitions from Terraform modules",
		Long:    learnLongDescription,
		Example: learnExample,
		Args:    cobra.NoArgs,
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringVar(&runner.GitURL, "git-url", "", "Git repository URL containing the Terraform module (required)")
	cmd.Flags().StringVar(&runner.Namespace, "namespace", "", "Namespace for the generated resource type (auto-inferred if not provided)")
	cmd.Flags().StringVar(&runner.TypeName, "type-name", "", "Name for the resource type (auto-generated if not provided)")
	cmd.Flags().StringVar(&runner.OutputFile, "output", "", "Output file path (prints to stdout if not provided)")

	_ = cmd.MarkFlagRequired("git-url")

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource-type learn` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	GitURL       string
	Namespace    string
	TypeName     string
	OutputFile   string
}

// NewRunner creates a new instance of the `rad resource-type learn` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource-type learn` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	if r.GitURL == "" {
		return fmt.Errorf("git-url is required")
	}
	return nil
}

// Run runs the `rad resource-type learn` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Learning resource type from Terraform module...")
	r.Output.LogInfo("Repository: %s", r.GitURL)

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "radius-terraform-learn-")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			r.Output.LogInfo("Warning: failed to clean up temporary directory: %v", err)
		}
	}()

	// Clone the git repository
	r.Output.LogInfo("Cloning repository...")
	modulePath, err := CloneGitRepository(r.GitURL, tempDir)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Parse the Terraform module
	r.Output.LogInfo("Parsing Terraform module...")
	module, err := ParseTerraformModule(modulePath)
	if err != nil {
		return fmt.Errorf("failed to parse Terraform module: %w", err)
	}

	if len(module.Variables) == 0 {
		r.Output.LogInfo("Warning: No Terraform variables found in the module")
	} else {
		r.Output.LogInfo("Found %d Terraform variables", len(module.Variables))
	}

	// Generate resource type name if not provided
	typeName := r.TypeName
	if typeName == "" {
		typeName = GenerateModuleName(r.GitURL)
		typeName = generateResourceTypeName(typeName)
		r.Output.LogInfo("Generated resource type name: %s", typeName)
	}

	// Infer namespace if not provided
	namespace := r.Namespace
	if namespace == "" {
		namespace = InferNamespaceFromModule(module, r.GitURL)
		r.Output.LogInfo("Inferred namespace: %s", namespace)
	}

	// Generate the resource type schema
	r.Output.LogInfo("Generating resource type schema...")
	schema, err := GenerateResourceTypeSchema(module, namespace, typeName)
	if err != nil {
		return fmt.Errorf("failed to generate resource type schema: %w", err)
	}

	// Convert to YAML
	yamlData, err := yaml.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Output the result
	if r.OutputFile != "" {
		// Create output directory if it doesn't exist
		outputDir := filepath.Dir(r.OutputFile)
		if err := os.MkdirAll(outputDir, fs.ModePerm); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		if err := os.WriteFile(r.OutputFile, yamlData, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		r.Output.LogInfo("Resource type definition written to: %s", r.OutputFile)
	} else {
		// Print to stdout
		fmt.Print(string(yamlData))
	}

	r.Output.LogInfo("Resource type learning completed successfully!")
	r.Output.LogInfo("You can now use 'rad resource-type create --from-file %s' to register this resource type", 
		func() string {
			if r.OutputFile != "" {
				return r.OutputFile
			}
			return "<saved-file>"
		}())

	return nil
}