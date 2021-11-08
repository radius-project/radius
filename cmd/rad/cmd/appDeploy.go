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
	"github.com/Azure/radius/pkg/cli/layers"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/radyaml"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

// appDeployCmd command to deploy based on a rad.yaml
var appDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a layer from a RAD application",
	Long:  "Deploy a layer from a RAD application in the current directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  deployApplication,
}

func init() {
	applicationCmd.AddCommand(appDeployCmd)

	appDeployCmd.Flags().Bool("all", false, "Deploy all layers")
	appDeployCmd.Flags().StringP("radfile", "r", "", "path to rad.yaml")
	appDeployCmd.Flags().StringP("target", "t", "", "target to build and deploy")
}

func deployApplication(cmd *cobra.Command, args []string) error {
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return err
	}

	radFile, err := cli.RequireRadYAML(cmd)
	if err != nil {
		return err
	}

	layer := ""
	if len(args) == 1 {
		layer = args[0]
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
	app, err := radyaml.Parse(file)
	if err != nil {
		return err
	}

	layersToProcess := app.Stages
	if layer != "" {
		for i := range app.Stages {
			if app.Stages[i].Name == layer {
				layersToProcess = app.Stages[0:i]
				break
			}
		}
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
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

	return layers.Process(cmd.Context(), env, target, baseDir, app, layersToProcess, all)
}
