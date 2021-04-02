// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/bicep"
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

	env, err := validateEnvironment()
	if err != nil {
		return err
	}

	step := logger.BeginStep("building application...")
	template, err := bicepBuild(filePath)
	if err != nil {
		return err
	}
	logger.CompleteStep(step)

	step = logger.BeginStep("deploying application...")
	err = deployApplication(cmd.Context(), template, env)
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
	ok, err := bicep.IsBicepInstalled()
	if err != nil {
		return "", fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	if !ok {
		logger.LogInfo("downloading bicep...")
		err = bicep.DownloadBicep()
		if err != nil {
			return "", fmt.Errorf("failed to download rad-bicep: %w", err)
		}
	}

	filepath, err := bicep.GetLocalBicepFilepath()
	if err != nil {
		return "", fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	// runs 'rad-bicep build' on the file
	//
	// rad-bicep is being told to output the template to stdout and we will capture it
	// rad-bicep will output compilation errors to stderr which will go to the user's console
	c := exec.Command(filepath, "build", "--stdout", filePath)
	c.Stderr = os.Stderr
	stdout, err := c.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create pipe: %w", err)
	}

	err = c.Start()
	if err != nil {
		return "", fmt.Errorf("rad-bicep build failed: %w", err)
	}

	// asyncronously copy to our buffer, we don't really need to observe
	// errors here since it's copying into memory
	buf := bytes.Buffer{}
	go func() {
		_, _ = io.Copy(&buf, stdout)
	}()

	// wait will wait for us to finish draining stderr before returning the exit code
	err = c.Wait()
	if err != nil {
		return "", fmt.Errorf("rad-bicep build failed: %w", err)
	}

	// read the content
	bytes, err := io.ReadAll(&buf)
	if err != nil {
		return "", fmt.Errorf("failed to read rad-bicep output: %w", err)
	}

	return string(bytes), err
}

func deployApplication(ctx context.Context, content string, env deployableEnvironment) error {
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

func createDeploymentClient(env deployableEnvironment) (resources.DeploymentsClient, error) {
	if env.Kind == "azure" {
		armauth, err := utils.GetResourceManagerEndpointAuthorizer()
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
