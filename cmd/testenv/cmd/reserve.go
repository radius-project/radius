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
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/spf13/cobra"
)

// The wait interval while we're polling for an available environment
const waitDuration = 30 * time.Second

// Table storage requires an explicit timeout (in seconds) on reads
const tableReadTimeout = 30

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

		timeout, err := cmd.Flags().GetDuration("timeout")
		if err != nil {
			return err
		}

		leaseTimeout, err := cmd.Flags().GetDuration("lease-timeout")
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()

		testenv, err := lease(ctx, accountName, accountKey, tableName, leaseTimeout)
		if err != nil {
			return err
		}

		v, err := cli.LoadConfig(configpath)
		if err != nil {
			return err
		}

		env, err := cli.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		// Note: the environment name is significant - it is the key to our storage table.
		// Most places in Radius environment names are just cosmetic, but for our tests
		// we're using it for tracking.
		env.Default = testenv.Name
		env.Items[testenv.Name] = map[string]interface{}{
			"kind":                      "azure",
			"subscriptionId":            testenv.SubscriptionID,
			"resourceGroup":             testenv.ResourceGroup,
			"controlPlaneResourceGroup": azure.GetControlPlaneResourceGroup(testenv.ResourceGroup),
			"clusterName":               testenv.ClusterName,
		}

		cli.UpdateEnvironmentSection(v, env)
		err = cli.SaveConfig(v)
		if err != nil {
			return err
		}

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

	reserveCmd.Flags().DurationP("timeout", "t", 60*time.Minute, "specifies wait timeout")
	err = reserveCmd.MarkFlagRequired("timeout")
	if err != nil {
		panic(err)
	}

	reserveCmd.Flags().Duration("lease-timeout", 120*time.Minute, "specifies duration an existing lease is considered valid")
	err = reserveCmd.MarkFlagRequired("lease-timeout")
	if err != nil {
		panic(err)
	}
}

func lease(ctx context.Context, accountName string, accountKey string, tableName string, leaseTimeout time.Duration) (*testEnvironment, error) {
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
		result, err := table.QueryEntities(tableReadTimeout, storage.MinimalMetadata, &storage.QueryOptions{})
		if err != nil {
			return nil, fmt.Errorf("cannot query table '%v': %w", tableName, err)
		}

		fmt.Println("scanning test environments...")
		available := findAvailableEnvironment(result, leaseTimeout)
		if available != nil {
			// We've taken the lease on this cluster, return it.
			return &testEnvironment{
				Name:           available.RowKey,
				SubscriptionID: available.Properties[PropertySubscriptionID].(string),
				ResourceGroup:  available.Properties[PropertyResourceGroup].(string),
				ClusterName:    available.Properties[PropertyClusterName].(string),
			}, nil
		}

		fmt.Println("waiting for a cluster to be available...")

		// If we get here then all clusters are busy.
		select {
		case <-time.After(waitDuration):
			fmt.Println("waking up...")
			continue
		case <-ctx.Done():
			// Cancelled or timed out
			return nil, fmt.Errorf("timed out waiting for a cluster to become available: %w", ctx.Err())
		}
	}
}

// We're just logging errors here instead of halting, we want to keep retrying.
func findAvailableEnvironment(response *storage.EntityQueryResult, leaseTimeout time.Duration) *storage.Entity {
	now := time.Now().UTC()
	for _, env := range response.Entities {
		fmt.Printf("checking environment '%v'...\n", env.RowKey)
		obj := env.Properties[PropertyReservedTime]

		if obj == nil || obj == "" {
			// not reserved
		} else {
			text, ok := obj.(string)
			if !ok {
				fmt.Printf("%s column should contain a string, was %T\n", PropertyReservedTime, obj)
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
		env.Properties[PropertyReservedTime] = now.Format(time.RFC3339)
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
	Name           string
	ResourceGroup  string
	SubscriptionID string
	ClusterName    string
}
