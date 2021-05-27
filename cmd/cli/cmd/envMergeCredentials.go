// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/rad/azcli"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/spf13/cobra"
)

// mergeCredentialsCmd represents the mergeCredentials command
var envMergeCredentialsCmd = &cobra.Command{
	Use:   "merge-credentials",
	Short: "Merge Kubernetes credentials",
	Long:  "Merge Kubernetes credentials into your local user store. Currently only supports Azure environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := requireEnvironment(cmd)

		isServicePrincipalConfigured, err := azure.IsServicePrincipalConfigured()
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

		err = azcli.RunCLICommand("aks", "get-credentials", "--subscription", env.SubscriptionID, "--resource-group", env.ResourceGroup, "--name", env.ClusterName)
		return err

	},
}

func init() {
	envCmd.AddCommand(envMergeCredentialsCmd)

	envMergeCredentialsCmd.Flags().StringP("name", "n", "", "The environment name")
}
