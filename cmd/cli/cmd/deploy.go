// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/version"
	"github.com/google/uuid"
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

	env, err := validateDefaultEnvironment()
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

	envUrl, err := azure.GenerateAzureEnvUrl(env.SubscriptionID, env.ResourceGroup)
	if err != nil {
		return err
	}

	step = logger.BeginStep("Deploying Application into environment '%v'...\n\n"+
		"Meanwhile, you can view the environment '%v' at:\n%v\n\n"+
		"Deployment In Progress...", env.Name, env.Name, envUrl)
	err = deployApplication(cmd.Context(), template, env)
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

func deployApplication(ctx context.Context, content string, env *environments.AzureCloudEnvironment) error {
	dc, err := createDeploymentClient(env)
	if err != nil {
		return err
	}

	template := map[string]interface{}{}
	err = json.Unmarshal([]byte(content), &template)
	if err != nil {
		return err
	}

	name := fmt.Sprintf("rad-deploy-%v", uuid.New().String())
	op, err := dc.CreateOrUpdate(ctx, env.ResourceGroup, name, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   template,
			Parameters: map[string]interface{}{},
			Mode:       resources.Incremental,
		},
	})
	if err != nil {
		return err
	}

	err = op.WaitForCompletionRef(ctx, dc.Client)
	if err != nil {
		return err
	}

	_, err = op.Result(dc)
	if err != nil {
		return err
	}

	return err
}

func createDeploymentClient(env *environments.AzureCloudEnvironment) (resources.DeploymentsClient, error) {
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return resources.DeploymentsClient{}, err
	}

	dc := resources.NewDeploymentsClient(env.SubscriptionID)
	dc.Authorizer = armauth

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	// Don't timeout, let the user cancel
	dc.PollingDuration = 0

	return dc, nil
}
