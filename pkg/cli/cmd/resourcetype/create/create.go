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

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

const (
	defaultPlaneName              = "local"
	msgNoResourceTypeNameProvided = "No resource type name provided. Creating all resource types in the manifest."
	msgAllResourceTypesCreated    = "All resource types in the manifest created successfully"
)

// NewCommand creates an instance of the `rad resource-type create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [resource-type-name]",
		Short: "Create or update a resource type",
		Long: `Create or update a resource type from a resource type definition file.

Resource types define the resources that Radius can deploy and the API for those resources. They are defined by a name, one or more API versions, and an OpenAPI schema. 

Input can be passed in using a JSON or YAML file using the --from-file option.

The resource type name argument is optional. If specified, only the specified type is created/updated. If not specified, all resource types in the referenced file are created/updated.

The resource type name argument is the simple name (e.g., 'testResources') not the fully qualified name.
`,
		Example: `
# Create a specific resource type from a YAML file
rad resource-type create myType --from-file /path/to/input.yaml

# Create a specific resource type from a JSON file
rad resource-type create myType --from-file /path/to/input.json

# Create all resource types from a YAML file
rad resource-type create --from-file /path/to/input.yaml
 
# Create all resource types from a JSON file
rad resource-type create --from-file /path/to/input.json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddFromFileFlagVar(cmd, &runner.ResourceProviderManifestFilePath)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resource-type create` command.
type Runner struct {
	UCPClientFactory *v20231001preview.ClientFactory
	ConfigHolder     *framework.ConfigHolder
	Output           output.Interface
	Format           string
	Workspace        *workspaces.Workspace

	ResourceProviderManifestFilePath string
	ResourceProvider                 *manifest.ResourceProvider
	ResourceTypeName                 string
	Logger                           func(format string, args ...any)
}

// NewRunner creates an instance of the runner for the `rad resource-type create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Logger: func(format string, args ...any) {
			output.LogInfo(format, args...)
		},
	}
}

// Validate runs validation for the `rad resource-type create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	resourceTypeName := cli.ReadResourceTypeNameArgs(cmd, args)
	r.ResourceTypeName = resourceTypeName

	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.ResourceProvider, err = manifest.ValidateManifest(cmd.Context(), r.ResourceProviderManifestFilePath)
	if err != nil {
		return err
	}

	resourcesTypes := r.ResourceProvider.Types
	if r.ResourceTypeName != "" {
		_, ok := resourcesTypes[r.ResourceTypeName]
		if !ok {
			return clierrors.Message("Resource type %q not found in the manifest", r.ResourceTypeName)
		}
	}

	return nil
}

// Run runs the `rad resource-type create` command.
func (r *Runner) Run(ctx context.Context) error {
	// Initialize the client factory if it hasn't been set externally.
	// This allows for flexibility where a test UCPClientFactory can be set externally during testing.
	if r.UCPClientFactory == nil {
		clientFactory, err := cmd.InitializeClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.UCPClientFactory = clientFactory
	}

	if r.ResourceTypeName == "" {
		r.Output.LogInfo(msgNoResourceTypeNameProvided)
		return r.registerTypes(ctx, nil) // Register all types
	}

	return r.registerTypes(ctx, []string{r.ResourceTypeName}) // Register single type
}

// registerTypes registers the specified resource types (or all types if typeNames is nil)
func (r *Runner) registerTypes(ctx context.Context, typeNames []string) error {
	// Always ensure the resource provider exists first
	err := manifest.EnsureResourceProviderExists(ctx, r.UCPClientFactory, defaultPlaneName, *r.ResourceProvider, r.Logger)
	if err != nil {
		return err
	}

	// Determine which types to register
	var typesToRegister []string
	if typeNames != nil {
		typesToRegister = typeNames
	} else {
		// Register all types in the manifest
		typesToRegister = make([]string, 0, len(r.ResourceProvider.Types))
		for typeName := range r.ResourceProvider.Types {
			typesToRegister = append(typesToRegister, typeName)
		}
	}

	// Register each type individually using the unified approach
	for _, typeName := range typesToRegister {
		err = manifest.RegisterType(ctx, r.UCPClientFactory, defaultPlaneName, r.ResourceProviderManifestFilePath, typeName, r.Logger)
		if err != nil {
			return err
		}
	}

	// Provide appropriate success message
	if len(typesToRegister) == 1 {
		// Single type - success message already logged by RegisterType
	} else {
		r.Output.LogInfo(msgAllResourceTypesCreated)
	}

	return nil
}
