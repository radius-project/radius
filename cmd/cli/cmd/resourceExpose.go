// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os/signal"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/radrp/schemav3"
	"github.com/spf13/cobra"
)

var resourceExposeCmd = &cobra.Command{
	Use:   "expose resource",
	Short: "Exposes a resource for network traffic",
	Long: `Exposes a port inside a resource for network traffic using a local port.
This command is useful for testing resources that accept network traffic but are not exposed to the public internet. Exposing a port for testing allows you to send TCP traffic from your local machine to the resource.

Press CTRL+C to exit the command and terminate the tunnel.`,
	Example: `# expose port 80 on the 'orders' resource of the 'icecream-store' application
# on local port 5000
rad resource expose --application icecream-store ContainerComponent orders --port 5000 --remote-port 80`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ConfigFromContext(cmd.Context())
		env, err := cli.RequireEnvironment(cmd, config)
		if err != nil {
			return err
		}

		application, err := cli.RequireApplication(cmd, env)
		if err != nil {
			return err
		}

		resourceType, resourceName, err := cli.RequireResource(cmd, args)
		if err != nil {
			return err
		}
		if resourceType != schemav3.ContainerComponentType {
			return fmt.Errorf("only %s is supported", schemav3.ContainerComponentType)
		}

		localPort, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}

		remotePort, err := cmd.Flags().GetInt("remote-port")
		if err != nil {
			return err
		}

		replica, err := cmd.Flags().GetString("replica")
		if err != nil {
			return err
		}

		if remotePort == -1 {
			remotePort = localPort
		}

		client, err := environments.CreateDiagnosticsClient(cmd.Context(), env)

		if err != nil {
			return err
		}

		failed, stop, signals, err := client.Expose(cmd.Context(), clients.ExposeOptions{
			Application: application,
			Component:   resourceName,
			Port:        localPort,
			RemotePort:  remotePort,
			Replica:     replica})

		if err != nil {
			return err
		}
		// We own stopping the signal created by Expose
		defer signal.Stop(signals)

		for {
			select {
			case <-signals:
				// shutting down... wait for socket to close
				close(stop)
				continue
			case err := <-failed:
				if err != nil {
					return fmt.Errorf("failed to port-forward: %w", err)
				}

				return nil
			}
		}
	},
}

func init() {
	resourceExposeCmd.PersistentFlags().StringP("type", "t", "", "The resource type")
	resourceExposeCmd.PersistentFlags().StringP("resource", "r", "", "The resource name")
	resourceExposeCmd.Flags().IntP("remote-port", "R", -1, "specify the remote port")
	resourceExposeCmd.Flags().String("replica", "", "specify the replica to expose")
	resourceExposeCmd.Flags().IntP("port", "p", -1, "specify the local port")
	err := resourceExposeCmd.MarkFlagRequired("port")
	if err != nil {
		panic(err)
	}
	resourceCmd.AddCommand(resourceExposeCmd)
}
