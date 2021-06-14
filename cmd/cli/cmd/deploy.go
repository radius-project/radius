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

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy [app.bicep]",
	Short: "Deploy a RAD application",
	Long:  "Deploy a RAD application",
	RunE:  deploy,
}

func init() {
	RootCmd.AddCommand(deployCmd)
	deployCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}

func deploy(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New(".bicep file is required")
	}

	filePath := args[0]
	err := validateBicepFile(filePath)
	if err != nil {
		return err
	}

	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	ok, err := bicep.IsBicepInstalled()
	if err != nil {
		return fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	if !ok {
		logger.LogInfo(fmt.Sprintf("Downloading Bicep for channel %s...", version.Channel()))
		err = bicep.DownloadBicep()
		if err != nil {
			return fmt.Errorf("failed to download rad-bicep: %w", err)
		}
	}

	step := logger.BeginStep("Building Application...")
	template, err := bicep.Build(filePath)
	if err != nil {
		return err
	}
	logger.CompleteStep(step)

	client, err := environments.CreateDeploymentClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	var progressText string
	status := env.GetStatusLink()
	if status == "" {
		progressText = fmt.Sprintf(
			"Deploying Application into environment '%v'...\n\n"+
				"Deployment In Progress...", env.GetName())
	} else {
		progressText = fmt.Sprintf(
			"Deploying Application into environment '%v'...\n\n"+
				"Meanwhile, you can view the environment '%v' at:\n%v\n\n"+
				"Deployment In Progress...", env.GetName(), env.GetName(), status)
	}

	step = logger.BeginStep(progressText)
	err = client.Deploy(cmd.Context(), template)
	if err != nil {
		return err
	}
	logger.CompleteStep(step)

	logger.LogInfo("Deployment Complete")

	return nil
}

func validateBicepFile(filePath string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("could not find file: %w", err)
	}

	if path.Ext(filePath) != ".bicep" {
		return errors.New("file must be a .bicep file")
	}

	return nil
}
