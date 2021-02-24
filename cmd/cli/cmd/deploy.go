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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy [app.bicep]",
	Short: "Deploy a RAD application",
	Long:  "Deploy a RAD application",
	Args:  cobra.ExactArgs(1),
	RunE:  deploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)
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

	env, err := validateEnvironment()
	if err != nil {
		return err
	}

	step := logger.BeginStep("building application...")
	compiledFilePath, err := bicepBuild(filePath)
	if err != nil {
		return err
	}
	logger.CompleteStep(step)

	step = logger.BeginStep("deploying application...")
	err = deployApplication(cmd.Context(), compiledFilePath, env)
	if err != nil {
		return err
	}
	logger.CompleteStep(step)

	logger.LogInfo("deployment complete")
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

func validateEnvironment() (deployableEnvironment, error) {
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return deployableEnvironment{}, err
	}

	if env.Default == "" {
		return deployableEnvironment{}, errors.New("no environment set, run 'rad env switch'")
	}

	e, ok := env.Items[env.Default]
	if !ok {
		return deployableEnvironment{}, fmt.Errorf("could not find environment: %v", env.Default)
	}

	kind := ""
	subscriptionID := ""
	resourceGroup := ""
	endpoint := ""

	value, ok := e["kind"].(string)
	if ok {
		kind = value
	}

	value, ok = e["subscriptionid"].(string)
	if ok {
		subscriptionID = value
	}

	value, ok = e["resourcegroup"].(string)
	if ok {
		resourceGroup = value
	}

	value, ok = e["endpoint"].(string)
	if ok {
		endpoint = value
	}

	de := deployableEnvironment{
		Kind:           kind,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		Endpoint:       endpoint,
	}

	if de.Kind != "azure" && de.Kind != "openarm" {
		return deployableEnvironment{}, fmt.Errorf("unsupported kind: %v. supported kinds are 'azure' and 'openarm'", de.Kind)
	}

	if de.Kind == "openarm" && de.Endpoint == "" {
		return deployableEnvironment{}, errors.New("endpoint is required for openarm environments")
	}

	if de.SubscriptionID == "" || de.ResourceGroup == "" {
		return deployableEnvironment{}, fmt.Errorf("subscriptionId and resourceGroup are required")
	}

	return de, nil
}

func bicepBuild(filePath string) (string, error) {
	var executableName string
	if runtime.GOOS == "windows" {
		executableName = "bicep.exe"
	} else {
		executableName = "bicep"
	}

	c := exec.Command(executableName, "build", filePath)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	if err != nil {
		return "", fmt.Errorf("bicep build failed: %w", err)
	}

	// produce filename with extension changed to '.json'
	var ext = path.Ext(filePath)
	return filePath[0:len(filePath)-len(ext)] + ".json", nil
}

func deployApplication(ctx context.Context, filePath string, env deployableEnvironment) error {
	dc, err := createDeploymentClient(env)
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	template := map[string]interface{}{}
	err = json.Unmarshal(b, &template)
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

func createDeploymentClient(env deployableEnvironment) (resources.DeploymentsClient, error) {
	if env.Kind == "azure" {
		settings, err := auth.GetSettingsFromEnvironment()
		if err != nil {
			return resources.DeploymentsClient{}, err
		}

		armauth, err := auth.NewAuthorizerFromCLIWithResource(settings.Environment.ResourceManagerEndpoint)
		if err != nil {
			return resources.DeploymentsClient{}, err
		}

		dc := resources.NewDeploymentsClient(env.SubscriptionID)
		dc.Authorizer = armauth
		return dc, nil
	} else if env.Kind == "openarm" {
		// no auth for now - #YOLO
		dc := resources.NewDeploymentsClient(env.SubscriptionID)
		dc.BaseURI = "http://" + env.Endpoint
		return dc, nil
	}

	panic(fmt.Sprintf("unexpected environment kind: %v", env.Kind))
}

type deployableEnvironment struct {
	Kind           string
	SubscriptionID string
	ResourceGroup  string
	Endpoint       string // REST endpoint for ARM - only used for openarm
}
