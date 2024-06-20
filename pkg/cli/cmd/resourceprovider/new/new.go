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

package new

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resourceprovider new` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "new",
		Short: "Scaffold new resource provider",
		Long: `Scaffold new resource provider

This command outputs a JSON template that can be used to create a new resource provider.
		
Resource providers are the entities that implement resource types such as 'Applications.Core/containers'. Resource providers can be defined, registered, and unregistered by users.`,
		Example: `
# Scaffold a new resource provider called 'Applications.Example'
rad resourceprovider new Applications.Example`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	cmd.Flags().BoolVar(&runner.Edit, "edit", false, "Open the new resource provider in the editor. Requires $EDITOR to be set.")
	cmd.Flags().BoolVarP(&runner.Force, "force", "f", false, "Overwrite existing file if present")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourceprovider new` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Format       string

	ResourceProviderNamespace string
	Edit                      bool
	Force                     bool
}

// NewRunner creates an instance of the runner for the `rad resourceprovider new` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resourceprovider new` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.ResourceProviderNamespace = args[0]

	return nil
}

// Run runs the `rad resourceprovider new` command.
func (r *Runner) Run(ctx context.Context) error {
	fileName := r.ResourceProviderNamespace + ".json"

	_, err := os.Stat(fileName)
	if errors.Is(err, os.ErrNotExist) {
		// Nothing to do, file doesn't exist.
	} else if err != nil {
		return err
	} else if !r.Force {
		return clierrors.Message("File %q already exists, use --force to overwrite.", fileName)
	}

	template := `{
  "location": "global",
  "properties": {
    "locations": {
      "global": {
        "address": "internal"
      }
    },
    "resourceTypes": [
      {
        "resourceType": "example",
        "routingType": "Internal",
        "locations": [
          "global"
        ],
        "apiVersions": {
          "2024-10-01-preview": {
            "schema": {
              "type": "object",
              "properties": {
                "message": {
                  "type": "string"
                }
              }
            }
          }
        },
        "capabilities": [
          "Recipe"
        ],
        "defaultApiVersion": "2024-10-01-preview"
      }
    ]
  }
}`

	err = os.WriteFile(fileName, []byte(template), 0644)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Wrote template to: %s", fileName)

	if r.Edit {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			r.Output.LogInfo("warning: EDITOR environment variable is not set")
			return nil
		}

		cmd := exec.CommandContext(ctx, editor, fileName)
		_ = cmd.Run() // Don't wait for command to finish
	}

	return nil
}
