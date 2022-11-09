// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os/signal"
	"strings"

	"github.com/agnivade/levenshtein"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	LevenshteinCutoff = 2

	ContainerType = "containers"
)

var resourceExposeCmd = &cobra.Command{
	Use:   "expose [type] [resource]",
	Short: "Exposes a resource for network traffic",
	Long: `Exposes a port inside a resource for network traffic using a local port.
This command is useful for testing resources that accept network traffic but are not exposed to the public internet. Exposing a port for testing allows you to send TCP traffic from your local machine to the resource.

Press CTRL+C to exit the command and terminate the tunnel.`,
	Example: `# expose port 80 on the 'orders' resource of the 'icecream-store' application
# on local port 5000
rad resource expose --application icecream-store containers orders --port 5000 --remote-port 80`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ConfigFromContext(cmd.Context())
		workspace, err := cli.RequireWorkspace(cmd, config)
		if err != nil {
			return err
		}

		// TODO: support fallback workspace
		if !workspace.IsNamedWorkspace() {
			return workspaces.ErrNamedWorkspaceRequired
		}

		// This gets the application name from the args provided
		application, err := cli.RequireApplication(cmd, *workspace)
		if err != nil {
			return err
		}

		//Check if the application provided exists or suggest a closest application in the scope
		managementClient, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
		if err != nil {
			return err
		}

		//ignore applicationresource as we only check for existence of application
		_, err = managementClient.ShowApplication(cmd.Context(), application)
		if err != nil {
			appNotFound := cli.Is404ErrorForAzureError(err)
			//suggest an application only when an existing one is not found
			if appNotFound {
				//ignore errors as we are trying to suggest an application and don't care about the errors in the suggestion process
				appList, listErr := managementClient.ListApplications(cmd.Context())
				if listErr != nil {
					return &cli.FriendlyError{Message: "Unable to list applications"}
				}
				msg := fmt.Sprintf("Application %s does not exist.", application)
				for _, app := range appList {
					distance := levenshtein.ComputeDistance(*app.Name, application)
					if distance <= LevenshteinCutoff {
						msg = msg + fmt.Sprintf("Did you mean %s?", *app.Name)
						break
					}
				}
				fmt.Println(msg)
			}
			return &cli.FriendlyError{Message: "Unable to expose resource"}
		}

		resourceType, resourceName, err := cli.RequireResource(cmd, args)
		if err != nil {
			return err
		}
		if !strings.EqualFold(resourceType, ContainerType) {
			return fmt.Errorf("only %s is supported", ContainerType)
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

		var client clients.DiagnosticsClient
		client, err = connections.DefaultFactory.CreateDiagnosticsClient(cmd.Context(), *workspace)

		if err != nil {
			return err
		}

		failed, stop, signals, err := client.Expose(cmd.Context(), clients.ExposeOptions{
			Application: application,
			Resource:    resourceName,
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
	resourceExposeCmd.Flags().IntP("remote-port", "", -1, "specify the remote port")
	resourceExposeCmd.Flags().String("replica", "", "specify the replica to expose")
	resourceExposeCmd.Flags().IntP("port", "p", -1, "specify the local port")
	err := resourceExposeCmd.MarkFlagRequired("port")
	if err != nil {
		panic(err)
	}
	resourceCmd.AddCommand(resourceExposeCmd)
}
