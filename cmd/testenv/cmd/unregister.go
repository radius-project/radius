// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/spf13/cobra"
)

var unregisterCmd = &cobra.Command{
	Use:   "unregister",
	Short: "Removes a test environment from the pool",
	Long:  `Removes a test environment from the pool.`,
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

		e, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		fmt.Printf("unregistering environment '%v'\n", e)

		err = unregister(cmd.Context(), accountName, accountKey, tableName, e)
		if err != nil {
			return err
		}

		fmt.Printf("unregistered environment '%v'\n", e)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(unregisterCmd)
	unregisterCmd.Flags().StringP("accountname", "a", "", "specifies storage account name")
	err := registerCmd.MarkFlagRequired("accountname")
	if err != nil {
		panic(err)
	}

	unregisterCmd.Flags().StringP("accountkey", "k", "", "specifies storage account key")
	err = registerCmd.MarkFlagRequired("accountkey")
	if err != nil {
		panic(err)
	}

	unregisterCmd.Flags().StringP("tablename", "n", "", "specifies storage account table")
	err = registerCmd.MarkFlagRequired("tablename")
	if err != nil {
		panic(err)
	}

	unregisterCmd.Flags().StringP("environment", "e", "", "specifies name of test environment to release")
	err = registerCmd.MarkFlagRequired("environment")
	if err != nil {
		panic(err)
	}
}

func unregister(ctx context.Context, accountName string, accountKey string, tableName string, env string) error {
	client, err := storage.NewBasicClient(accountName, accountKey)
	if err != nil {
		return fmt.Errorf("failed to authenticate with table storage: %w", err)
	}

	tableClient := client.GetTableService()
	table := tableClient.GetTableReference(tableName)
	if table == nil {
		return fmt.Errorf("could not find table '%v'", tableName)
	}

	entity := table.GetEntityReference(env, env)
	err = entity.Delete(true, &storage.EntityOptions{})
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to delete entity: %w", err)
	}

	return nil
}
