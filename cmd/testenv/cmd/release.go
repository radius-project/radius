// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Reserves a test environment to the pool",
	Long:  `Reserves a test environment to the pool. Will release the environment specified or named by the configuration file.`,
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

		if (configpath == "" && e == "") || (configpath != "" && e != "") {
			return errors.New("one of configpath or environment must be specified")
		}

		// Note: the environment name is significant - it is the key to our storage table.
		// Most places in Radius environment names are just cosmetic, but for our tests
		// we're using it for tracking.
		if configpath != "" {
			azureenv, err := readEnvironmentFromConfigfile(configpath, "")
			if err != nil {
				return err
			}

			e = azureenv.Name
		}

		fmt.Printf("releasing environment '%v'\n", e)

		err = release(cmd.Context(), accountName, accountKey, tableName, e)
		if err != nil {
			return err
		}

		fmt.Printf("released environment '%v'\n", e)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(releaseCmd)
	releaseCmd.Flags().StringP("accountname", "a", "", "specifies storage account name")
	err := releaseCmd.MarkFlagRequired("accountname")
	if err != nil {
		panic(err)
	}

	releaseCmd.Flags().StringP("accountkey", "k", "", "specifies storage account key")
	err = releaseCmd.MarkFlagRequired("accountkey")
	if err != nil {
		panic(err)
	}

	releaseCmd.Flags().StringP("tablename", "n", "", "specifies storage account table")
	err = releaseCmd.MarkFlagRequired("tablename")
	if err != nil {
		panic(err)
	}

	releaseCmd.Flags().StringP("configpath", "t", "", "specifies location to write config")
	releaseCmd.Flags().StringP("environment", "e", "", "specifies name of test environment to release")
}

func readEnvironmentFromConfigfile(configpath string, name string) (*environments.AzureCloudEnvironment, error) {
	v, err := cli.LoadConfig(configpath)
	if err != nil {
		return nil, err
	}

	env, err := cli.ReadEnvironmentSection(v)
	if err != nil {
		return nil, err
	}

	testenv, err := env.GetEnvironment(name)
	if err != nil {
		return nil, err
	}

	az, err := environments.RequireAzureCloud(testenv)
	if err != nil {
		return nil, err
	}

	return az, nil
}

func release(ctx context.Context, accountName string, accountKey string, tableName string, environmentName string) error {
	client, err := storage.NewBasicClient(accountName, accountKey)
	if err != nil {
		return fmt.Errorf("failed to authenticate with table storage: %w", err)
	}

	tableClient := client.GetTableService()
	table := tableClient.GetTableReference(tableName)
	if table == nil {
		return fmt.Errorf("could not find table '%v'", tableName)
	}

	entity := table.GetEntityReference(environmentName, environmentName)
	if entity == nil {
		return fmt.Errorf("could not find entry for environment '%v'", environmentName)
	}

	err = entity.Get(tableReadTimeout, storage.MinimalMetadata, &storage.GetEntityOptions{})
	if err != nil {
		return fmt.Errorf("failed to read entity: %w", err)
	}

	entity.Properties["ReservedTime"] = ""
	return entity.Merge(false, &storage.EntityOptions{})
}
