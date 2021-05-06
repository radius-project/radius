// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a test environment with the pool",
	Long:  `Registers a test environment with the pool. Will register the environment specified by name by the configuration file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		accountName, err := cmd.Flags().GetString("accountname")
		if err != nil {
			return err
		}

		accountKey, err := cmd.Flags().GetString("accountkey")
		if err != nil {
			return err
		}

		tableName, err := cmd.Flags().GetString("tablename")
		if err != nil {
			return err
		}

		configpath, err := cmd.Flags().GetString("configpath")
		if err != nil {
			return err
		}

		e, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		// Note: the environment name is significant - it is the key to our storage table.
		// Most places in Radius environment names are just cosmetic, but for our tests
		// we're using it for tracking.
		azureenv, err := readEnvironmentFromConfigfile(configpath, e)
		if err != nil {
			return err
		}

		fmt.Printf("registering environment '%v'\n", azureenv.Name)

		err = register(cmd.Context(), accountName, accountKey, tableName, azureenv)
		if err != nil {
			return err
		}

		fmt.Printf("registered environment '%v'\n", e)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(registerCmd)
	registerCmd.Flags().StringP("accountname", "a", "", "specifies storage account name")
	err := registerCmd.MarkFlagRequired("accountname")
	if err != nil {
		panic(err)
	}

	registerCmd.Flags().StringP("accountkey", "k", "", "specifies storage account key")
	err = registerCmd.MarkFlagRequired("accountkey")
	if err != nil {
		panic(err)
	}

	registerCmd.Flags().StringP("tablename", "n", "", "specifies storage account table")
	err = registerCmd.MarkFlagRequired("tablename")
	if err != nil {
		panic(err)
	}

	registerCmd.Flags().StringP("environment", "e", "", "specifies name of test environment to release")
	err = registerCmd.MarkFlagRequired("environment")
	if err != nil {
		panic(err)
	}

	registerCmd.Flags().StringP("configpath", "t", "", "specifies location to write config")
}

func register(ctx context.Context, accountName string, accountKey string, tableName string, env *environments.AzureCloudEnvironment) error {
	client, err := storage.NewBasicClient(accountName, accountKey)
	if err != nil {
		return fmt.Errorf("failed to authenticate with table storage: %w", err)
	}

	tableClient := client.GetTableService()
	table := tableClient.GetTableReference(tableName)
	if table == nil {
		return fmt.Errorf("could not find table '%v'", tableName)
	}

	entity := table.GetEntityReference(env.Name, env.Name)
	entity.Properties = map[string]interface{}{
		"CreatedTime":    time.Now().UTC().Format(time.RFC3339),
		"ReservedTime":   "",
		"subscriptionId": env.SubscriptionID,
		"resourceGroup":  env.ResourceGroup,
		"clusterName":    env.ClusterName,
	}

	err = entity.Insert(storage.MinimalMetadata, &storage.EntityOptions{})
	if err != nil {
		return fmt.Errorf("failed to insert entity: %w", err)
	}

	return nil
}
