// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/web/mgmt/web"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azcli"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		configpath, err := cmd.Flags().GetString("configpath")
		if err != nil {
			return err
		}

		v := viper.GetViper()
		v.SetConfigFile(configpath)
		err = v.ReadInConfig()
		if err != nil {
			return err
		}

		env, err := rad.ReadEnvironmentSection(v)
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

		auth, _, err := armauth.GetArmAuthorizerAndClientID()
		if err != nil {
			return err
		}

		fmt.Printf("updating environment '%v' to use '%v'\n", az.ResourceGroup, image)

		err = updateRP(cmd.Context(), *auth, *az, image)
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

	updateRPCmd.Flags().StringP("configpath", "t", "", "specifies location to write config")
	err = updateRPCmd.MarkFlagRequired("configpath")
	if err != nil {
		panic(err)
	}
}

func updateRP(ctx context.Context, auth autorest.Authorizer, env environments.AzureCloudEnvironment, image string) error {
	webc := web.NewAppsClient(env.SubscriptionID)
	webc.Authorizer = auth

	list, err := webc.ListByResourceGroupComplete(ctx, env.ResourceGroup, nil)
	if err != nil {
		return fmt.Errorf("cannot read web sites: %w", err)
	}

	if !list.NotDone() {
		return fmt.Errorf("failed to find website in resource group '%v'", env.ResourceGroup)
	}

	website := *list.Value().Name
	fmt.Printf("found website '%v' in resource group '%v'", website, env.ResourceGroup)

	// This command will update the deployed image
	args := []string{
		"webapp", "config", "container", "set",
		"--resource-group", env.ResourceGroup,
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
		"--resource-group", env.ResourceGroup,
		"--subscription", env.SubscriptionID,
		"--name", website,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to restart rp: %w", err)
	}

	return nil
}
