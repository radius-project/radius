// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azcli"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
)

// mergeCredentialsCmd represents the mergeCredentials command
var envMergeCredentialsCmd = &cobra.Command{
	Use:   "merge-credentials",
	Short: "Merge Kubernetes credentials",
	Long:  "Merge Kubernetes credentials into your local user store. Currently only supports Azure environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ConfigFromContext(cmd.Context())
		env, err := cli.RequireEnvironmentArgs(cmd, config, args)
		if err != nil {
			return err
		}

		isServicePrincipalConfigured, err := armauth.IsServicePrincipalConfigured()
		if err != nil {
			return err
		}

		if isServicePrincipalConfigured {
			settings, err := auth.GetSettingsFromEnvironment()
			if err != nil {
				return fmt.Errorf("could not read environment settings")
			}

			err = azcli.RunCLICommand("login", "--service-principal", "--username", settings.Values[auth.ClientID], "--password", settings.Values[auth.ClientSecret], "--tenant", settings.Values[auth.TenantID])
			if err != nil {
				return err
			}
		}

		az, err := environments.RequireAzureCloud(env)
		if err != nil {
			return err
		}

		err = azcli.RunCLICommand("aks", "get-credentials", "--subscription", az.SubscriptionID, "--resource-group", az.ResourceGroup, "--name", az.ClusterName)
		return err

	},
}

func init() {
	envCmd.AddCommand(envMergeCredentialsCmd)
}
