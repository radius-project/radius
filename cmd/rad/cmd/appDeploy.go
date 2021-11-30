// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/radyaml"
	"github.com/Azure/radius/pkg/cli/stages"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

// appDeployCmd command to deploy based on a rad.yaml
var appDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a RAD application using rad.yaml",
	Long: `Deploy a RAD application using rad.yaml

The app deploy command reads a rad.yaml file to process a series of deployment stages. For example a typical
application will have an 'infra' (infrastructure) phase followed by an 'app' (application code) phase.
'rad app deploy'  will deploy both stages in sequence and automatically pass the outputs of the 'infra' 
phase to the 'app' phase as parameters.

By default 'rad app deploy' will run all stages. You can specify a stage at the command line to control
which stages run. If you specify a stage at the command line all stages before and including the specified
stage are run. Stages after the specified stage are skipped.

You can specify parameters at the command line using the '--parameter' flag ('-p' for short). Parameters
specified at the command line apply to all stages and must therefore exist in the template file for every stage. 

Parameters can be passed as:

- A file containing multiple parameters using the ARM JSON parameter format (see below)
- A file containing a single value in JSON format
- A key-value-pair passed in the command line

When passing multiple parameters in a single file, use the format described here:

	https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files

You can specify parameters using multiple sources. Parameters can be overridden based on the 
order the are provided. Parameters appearing later in the argument list will override those defined earlier.
`,
	Example: `
# deploy the application (basic)

rad app deploy


# deploy to a specific environment

rad app deploy --environment production


# specify the 'infra' stage

rad app deploy infra --parameters version=latest


# specify a string parameter

rad app deploy --parameters version=latest


# specify a non-string parameter using a JSON file

rad app deploy --parameters configuration=@myfile.json


# specify many parameters using an ARM JSON parameter file

rad app deploy --parameters @myfile.json


# specify parameters from multiple sources

rad app deploy --parameters @myfile.json --parameters version=latest
`,
	Args: cobra.MaximumNArgs(1),
	RunE: deployApplication,
}

func init() {
	applicationCmd.AddCommand(appDeployCmd)
	appDeployCmd.Flags().StringP("environment", "e", "", "The environment name")
	appDeployCmd.Flags().StringP("radfile", "r", "", "The path to rad.yaml. The default is './rad/rad.yaml'")
	appDeployCmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")
}

func deployApplication(cmd *cobra.Command, args []string) error {
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

	parser := cli.ParameterParser{FileSystem: cli.OSFileSystem{}}
	parameters, err := parser.Parse(parameterArgs)
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
		Parameters:    parameters,
		FinalStage:    stage,
	}

	_, err = stages.Run(cmd.Context(), options)
	if err != nil {
		return err
	}

	return nil
}
