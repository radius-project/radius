// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// mergeCredentialsCmd represents the mergeCredentials command
var envMergeCredentialsCmd = &cobra.Command{
	Use:   "merge-credentials",
	Short: "Merge Kubernetes credentials",
	Long:  "Merge Kubernetes credentials into your local user store. Currently only supports Azure environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := cmd.Flags().GetString("name")
		if err != nil {
			return err
		}

		if name == "" {
			return errors.New("name is required")
		}

		v := viper.GetViper()
		section, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		props, ok := section.Items[name]
		if !ok {
			return fmt.Errorf("environment %v not found", name)
		}

		val, ok := props["kind"]
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		kind, ok := val.(string)
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		if kind != "azure" {
			return errors.New("merge-credentials only supports Azure environments (for now...)")
		}

		val, ok = props["clustername"]
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		clusterName, ok := val.(string)
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		val, ok = props["resourcegroup"]
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		resourceGroup, ok := val.(string)
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		val, ok = props["subscriptionid"]
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		subscriptionID, ok := val.(string)
		if !ok {
			return fmt.Errorf("could not read environment %v", name)
		}

		var executableName string
		if runtime.GOOS == "windows" {
			executableName = "az.exe"
		} else {
			executableName = "az"
		}

		isServicePrincipalConfigured, err := utils.IsServicePrincipalConfigured()
		if err != nil {
			return err
		}

		if isServicePrincipalConfigured {
			settings, err := auth.GetSettingsFromEnvironment()
			if err != nil {
				return fmt.Errorf("could not read environment settings")
			}
			c := exec.Command(executableName, "login", "--service-principal", "--username", settings.Values[auth.ClientID], "--password", settings.Values[auth.ClientSecret], "--tenant", settings.Values[auth.TenantID])
			c.Stderr = os.Stderr
			c.Stdout = os.Stdout
			err = c.Run()
			if err != nil {
				return err
			}
		}

		c := exec.Command(executableName, "aks", "get-credentials", "--subscription", subscriptionID, "--resource-group", resourceGroup, "--name", clusterName)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err = c.Run()
		return err

	},
}

func init() {
	envCmd.AddCommand(envMergeCredentialsCmd)

	envMergeCredentialsCmd.Flags().String("name", "", "The environment name")
}
