// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azcli"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/version"
	"github.com/spf13/cobra"
)

const (
	VersionQueryAttempts = 10
	VersionQueryDelay    = 10 * time.Second
)

var updateRPCmd = &cobra.Command{
	Use:   "update-rp",
	Short: "Updates a test environment to use a specific container image",
	Long:  `Updates a test environment to use a specific container image. Updates the environment specified by the provided config file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		image, err := cmd.Flags().GetString("image")
		if err != nil {
			return err
		}

		checkVersion, err := cmd.Flags().GetString("check-version")
		if err != nil {
			return err
		}

		configpath, err := cmd.Flags().GetString("configpath")
		if err != nil {
			return err
		}

		v, err := cli.LoadConfig(configpath, true)
		if err != nil {
			return err
		}

		env, err := cli.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		testenv, err := env.GetEnvironment("")
		if err != nil {
			return err
		}

		az, err := environments.RequireAzureCloud(testenv)
		if err != nil {
			return err
		}

		auth, err := armauth.GetArmAuthorizer()
		if err != nil {
			return err
		}

		fmt.Printf("updating environment '%v' to use '%v'\n", az.ResourceGroup, image)

		err = updateRP(cmd.Context(), auth, *az, image, checkVersion)
		if err != nil {
			return err
		}

		fmt.Printf("updated environment '%v' to use '%v'\n", az.ResourceGroup, image)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(updateRPCmd)
	updateRPCmd.Flags().StringP("image", "i", "", "image to use")
	err := updateRPCmd.MarkFlagRequired("image")
	if err != nil {
		panic(err)
	}

	updateRPCmd.Flags().String("check-version", "", "image version to check")

	updateRPCmd.Flags().StringP("configpath", "t", "", "specifies location to write config")
	err = updateRPCmd.MarkFlagRequired("configpath")
	if err != nil {
		panic(err)
	}
}

func updateRP(ctx context.Context, auth autorest.Authorizer, env environments.AzureCloudEnvironment, image string, checkVersion string) error {
	webc := clients.NewWebClient(env.SubscriptionID, auth)

	list, err := webc.ListByResourceGroupComplete(ctx, env.ControlPlaneResourceGroup, nil)
	if err != nil {
		return fmt.Errorf("cannot read web sites: %w", err)
	}

	if !list.NotDone() {
		return fmt.Errorf("failed to find website in resource group '%v'", env.ControlPlaneResourceGroup)
	}

	website := *list.Value().Name
	fmt.Printf("found website '%v' in resource group '%v'", website, env.ControlPlaneResourceGroup)

	// This command will update the deployed image
	args := []string{
		"webapp", "config", "container", "set",
		"--resource-group", env.ControlPlaneResourceGroup,
		"--subscription", env.SubscriptionID,
		"--name", website,
		"--docker-custom-image-name", image,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to update container to %v: %w", image, err)
	}

	// This command will restart the webapp
	args = []string{
		"webapp", "restart",
		"--resource-group", env.ControlPlaneResourceGroup,
		"--subscription", env.SubscriptionID,
		"--name", website,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to restart rp: %w", err)
	}

	if checkVersion != "" {
		fmt.Printf("checking for release version: %s\n", checkVersion)
		url := fmt.Sprintf("https://%s.azurewebsites.net/version", website)
		fmt.Printf("querying website version at: %s\n", url)

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				// We need this to talk to app service for some reason.
				// https://stackoverflow.com/questions/57420833/tls-no-renegotiation-error-on-http-request
				Renegotiation: tls.RenegotiateOnceAsClient,
			},
		}

		client := &http.Client{
			Transport: transport,
		}

		maxTryCount := 10
		for tryCount := 1; tryCount <= maxTryCount; tryCount++ {
			info, err := queryVersion(client, url)
			if err != nil {
				if tryCount == maxTryCount {
					return fmt.Errorf("failed to query version: %w", err)
				} else {
					fmt.Println("error: " + err.Error())
					fmt.Printf("waiting %s\n", VersionQueryDelay)
					time.Sleep(VersionQueryDelay)
					continue
				}
			}

			if info.Release != checkVersion {
				fmt.Printf("mismatched version - expected: %s actual: %+v\n", checkVersion, info)
				fmt.Printf("waiting %s\n", VersionQueryDelay)
				time.Sleep(VersionQueryDelay)
				continue
			}

			fmt.Printf("found version match - expected: %s actual: %+v\n", checkVersion, info)
			break
		}
	}

	return nil
}

func queryVersion(client *http.Client, url string) (*version.VersionInfo, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query version: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("response status was: %d", response.StatusCode)
	}

	decoder := json.NewDecoder(response.Body)
	defer response.Body.Close()

	info := version.VersionInfo{}
	err = decoder.Decode(&info)
	if err != nil {
		return nil, fmt.Errorf("failed to read response payload: %w", err)
	}

	return &info, nil
}
