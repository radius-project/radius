// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/builders"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/radyaml"
	"github.com/project-radius/radius/pkg/cli/stages"
	"github.com/project-radius/radius/pkg/cli/tools"
	"github.com/project-radius/radius/pkg/version"
	"github.com/spf13/cobra"
)

// appRunCmd command deploys an application interactively based on a rad.yaml
var appRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a Radius application using rad.yaml",
	Long: `Run a Radius application using rad.yaml

The app run command uses a 'rad.yaml' config to deploy an application for development purposes. 'rad app run'
is similar to 'rad app deploy' except it uses the console to stream logs after deployment until the command
is cancelled. See the documentation of 'rad app deploy' for a full description of the command line options
to affect deployment.
`,
	Example: `
# run the application (basic)

rad app run
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runApplication,
}

func init() {
	applicationCmd.AddCommand(appRunCmd)
	appRunCmd.Flags().StringP("environment", "e", "", "The environment name")
	appRunCmd.Flags().StringP("radfile", "r", "", "The path to rad.yaml. The default is './rad.yaml'")
	appRunCmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")
	appRunCmd.Flags().String("profile", "", "Specify profile of the application for deployment")
}

func runApplication(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	radFile, err := cli.RequireRadYAML(cmd)
	if err != nil {
		return err
	}

	stage := ""
	if len(args) == 1 {
		stage = args[0]
	}

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	profile, err := cmd.Flags().GetString("profile")
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: bicep.OSFileSystem{}}
	parameters, err := parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	output.LogInfo("Reading %s...", radFile)

	file, err := os.Open(radFile)
	if err == os.ErrNotExist {
		return fmt.Errorf("could not find rad.yaml at %q", radFile)
	} else if err != nil {
		return err
	}
	defer file.Close()

	baseDir := path.Dir(radFile)
	manifest, err := radyaml.Parse(file)
	if err != nil {
		return err
	}

	ok, err := bicep.IsBicepInstalled()
	if err != nil {
		return fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	if !ok {
		output.LogInfo(fmt.Sprintf("Downloading Bicep for channel %s...", version.Channel()))
		err = bicep.DownloadBicep()
		if err != nil {
			return fmt.Errorf("failed to download rad-bicep: %w", err)
		}
	}

	options := stages.Options{
		Environment:   env,
		BaseDirectory: baseDir,
		Manifest:      manifest,
		Builders:      builders.DefaultBuilders(),
		Parameters:    parameters,
		Profile:       profile,
		FinalStage:    stage,

		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	_, err = stages.Run(cmd.Context(), options)
	if err != nil {
		return err
	}

	output.LogInfo("Application %s has been deployed. Press CTRL+C to exit...", manifest.Name)
	output.LogInfo("")

	if dev, ok := env.(*environments.LocalEnvironment); ok {
		output.LogInfo("Launching log stream...")
		err = tools.SternStart(cmd.Context(), dev.Context, dev.Namespace, manifest.Name)
		if errors.As(err, &tools.ErrToolNotFound{}) {
			output.LogInfo(err.Error()) // Tolerate missing tool
		} else if err != nil {
			return err
		}
	}

	// Block until cancelled.
	<-cmd.Context().Done()
	return nil
}
