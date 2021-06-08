// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os/signal"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/clients"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

var exposeCmd = &cobra.Command{
	Use:   "expose component",
	Short: "Exposes a component for network traffic",
	Long: `Exposes a port inside a component for network traffic using a local port.
This command is useful for testing components that accept network traffic but are not exposed to the public internet. Exposing a port for testing allows you to send TCP traffic from your local machine to the component.

Press CTRL+C to exit the command and terminate the tunnel.`,
	Example: `# expose port 80 on the 'orders' component of the 'icecream-store' application
# on local port 5000
rad component expose --application icecream-store orders --port 5000 --remote-port 80`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := rad.RequireEnvironment(cmd)
		if err != nil {
			return err
		}

		application, err := rad.RequireApplication(cmd, env)
		if err != nil {
			return err
		}

		component, err := rad.RequireComponent(cmd, args)
		if err != nil {
			return err
		}

		localPort, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}

		remotePort, err := cmd.Flags().GetInt("remote-port")
		if err != nil {
			return err
		}

		if remotePort == -1 {
			remotePort = localPort
		}

		client, err := environments.CreateDiagnosticsClient(env)

		if err != nil {
			return err
		}

		failed, stop, signals, err := client.Expose(cmd.Context(), clients.ExposeOptions{
			Application: application,
			Component:   component,
			Port:        localPort,
			RemotePort:  remotePort})

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
	componentCmd.AddCommand(exposeCmd)

	exposeCmd.Flags().IntP("port", "p", -1, "specify the local port")
	err := exposeCmd.MarkFlagRequired("port")
	if err != nil {
		panic(err)
	}

	exposeCmd.Flags().IntP("remote-port", "r", -1, "specify the remote port")
}
