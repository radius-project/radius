/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"os/signal"
	"strings"

	"github.com/agnivade/levenshtein"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
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
		workspace, err := cli.RequireWorkspace(cmd, ConfigFromContext(cmd.Context()), DirectoryConfigFromContext(cmd.Context()))
		if err != nil {
			return err
		}

		scope, err := cli.RequireScope(cmd, *workspace)
		if err != nil {
			return err
		}
		workspace.Scope = scope

		// This gets the application name from the args provided
		application, err := cli.RequireApplication(cmd, *workspace)
		if err != nil {
			return err
		}

		// Check if the application provided exists or suggest a closest application in the scope
		managementClient, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
		if err != nil {
			return err
		}

		// Ignore applicationresource as we only check for existence of application
		_, err = managementClient.ShowApplication(cmd.Context(), application)
		if err != nil {
			appNotFound := clients.Is404Error(err)
			if !appNotFound {
				return clierrors.MessageWithCause(err, "Unable to find application %s.", application)
			}

			// Suggest an application only when an existing one is not found.
			// Ignore errors as we are trying to suggest an application and don't care about the errors in the suggestion process.
			appList, err := managementClient.ListApplications(cmd.Context())
			if err != nil {
				return clierrors.MessageWithCause(err, "Unable to list applications.")
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
	commonflags.AddResourceGroupFlag(resourceExposeCmd)
	err := resourceExposeCmd.MarkFlagRequired("port")
	if err != nil {
		panic(err)
	}
	resourceCmd.AddCommand(resourceExposeCmd)
}
