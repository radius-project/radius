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
	"github.com/Azure/radius/pkg/rad"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const leaseTimeout = 30 * time.Minute

var reserveCmd = &cobra.Command{
	Use:   "reserve",
	Short: "Reserves a test environment and updates rad environment config",
	Long:  `Reserves a test environment and updates rad environment config`,
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

		timeout, err := cmd.Flags().GetInt("timeout")
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*time.Duration(timeout))
		defer cancel()

		testenv, err := lease(ctx, accountName, accountKey, tableName)
		if err != nil {
			return err
		}

		v := viper.GetViper()
		env, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		env.Default = "test"
		env.Items["test"] = map[string]interface{}{
			"kind":           "azure",
			"subscriptionId": testenv.SubscriptionID,
			"resourceGroup":  testenv.ResourceGroup,
			"clusterName":    testenv.ClusterName,
		}

		rad.UpdateEnvironmentSection(v, env)
		err = v.SafeWriteConfigAs(configpath)
		if err != nil {
			return err
		}

		fmt.Printf("wrote config to '%v'\n", configpath)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(reserveCmd)
	reserveCmd.Flags().StringP("accountname", "a", "", "specifies storage account name")
	err := reserveCmd.MarkFlagRequired("accountname")
	if err != nil {
		panic(err)
	}

	reserveCmd.Flags().StringP("accountkey", "k", "", "specifies storage account key")
	err = reserveCmd.MarkFlagRequired("accountkey")
	if err != nil {
		panic(err)
	}

	reserveCmd.Flags().StringP("tablename", "n", "", "specifies storage account table")
	err = reserveCmd.MarkFlagRequired("tablename")
	if err != nil {
		panic(err)
	}

	reserveCmd.Flags().StringP("configpath", "c", "", "specifies location to write config")
	err = reserveCmd.MarkFlagRequired("configpath")
	if err != nil {
		panic(err)
	}

	reserveCmd.Flags().IntP("timeout", "t", 30, "specifies wait timeout in minutes")
	err = reserveCmd.MarkFlagRequired("timeout")
	if err != nil {
		panic(err)
	}
}

func lease(ctx context.Context, accountName string, accountKey string, tableName string) (*testEnvironment, error) {
	client, err := storage.NewBasicClient(accountName, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with table storage: %w", err)
	}

	tableClient := client.GetTableService()
	table := tableClient.GetTableReference(tableName)
	if table == nil {
		return nil, fmt.Errorf("could not find table '%v'", tableName)
	}

	// Repeat until we can find a test environment that's available
	for {
		// list all entities - this will list all of the test environments
		result, err := table.QueryEntities(30, storage.MinimalMetadata, &storage.QueryOptions{})
		if err != nil {
			return nil, fmt.Errorf("cannot query table '%v': %w", tableName, err)
		}

		fmt.Println("scanning test environments...")
		available := findAvailableEnvironment(result)
		if available != nil {
			// We've taken the lease on this cluster, return it.
			return &testEnvironment{
				SubscriptionID: available.Properties["subscriptionId"].(string),
				ResourceGroup:  available.Properties["resourceGroup"].(string),
				ClusterName:    available.Properties["clustername"].(string),
			}, nil
		}

		fmt.Println("waiting for a cluster to be available...")

		// If we get here then all clusters are busy.
		select {
		case <-time.After(30 * time.Second):
			fmt.Println("waking up...")
			continue
		case <-ctx.Done():
			// Cancelled or timed out
			return nil, fmt.Errorf("timed out waiting for a cluster to become available: %w", ctx.Err())
		}
	}
}

// We're just logging errors here instead of halting, we want to keep retrying.
func findAvailableEnvironment(response *storage.EntityQueryResult) *storage.Entity {
	now := time.Now().UTC()
	for _, env := range response.Entities {
		fmt.Printf("checking environment '%v'...\n", env.RowKey)
		obj := env.Properties["reservedat"]

		if obj == nil || obj == "" {
			// not reserved
		} else {
			text, ok := obj.(string)
			if !ok {
				fmt.Printf("reservedat column should contain a string, was %T\n", obj)
				continue
			}

			reserved, err := time.Parse(time.RFC3339, text)
			if err != nil {
				fmt.Printf("could not parse timestamp '%v': %v\n", obj, err)
				continue
			}

			if now.Sub(reserved.UTC()) <= leaseTimeout {
				next := reserved.UTC().Add(leaseTimeout).Local()
				fmt.Printf("environment '%v' is still reserved until at least: %v", env.RowKey, next)
				continue
			}

			fmt.Printf("considering %v to be available based on lease expiry\n", env.RowKey)
		}

		// We're ok to take this one, try to take it atomically.
		env.Properties["reservedat"] = now.Format(time.RFC3339)
		err := env.Merge(false, &storage.EntityOptions{})
		if err != nil {
			fmt.Printf("failed to take lease on '%v': %v\n", env.RowKey, err)
		}

		return env
	}

	// none available :(
	return nil
}

type testEnvironment struct {
	ResourceGroup  string
	SubscriptionID string
	ClusterName    string
}
