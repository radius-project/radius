// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/clients"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [application] [component]",
	Short: "Read logs from a running container component",
	Long: `Reads logs from a running component. Currently only supports the kind 'radius.dev/Container'.
This command allows you to access logs of a deployed application and output those logs to the local console.

'rad component logs' will output logs from the component's primary container. In scenarios like Dapr where multiple containers are in use, the '--continer <name>' option can specify the desired container.

'rad component logs' will output all currently available logs for the component and then exit.

Specify the '--follow' option to stream additional logs as they are emitted by the component. When following, press CTRL+C to exit the command and terminate the stream.`,
	Example: `# read logs from the 'orders' component of the 'icecream-store' application
rad component logs --application icecream-store orders

# stream logs from the 'orders' component of the 'icecream-store' application
rad component logs --application icecream-store orders --follow

# read logs from the 'daprd' sidecare container of the 'orders' component of the 'icecream-store' application
rad component logs --application icecream-store orders --container daprd`,
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

		follow, err := cmd.Flags().GetBool("follow")
		if err != nil {
			return err
		}

		container, err := cmd.Flags().GetString("container")
		if err != nil {
			return err
		}

		client, err := environments.CreateDiagnosticsClient(env)
		if err != nil {
			return err
		}

		return client.Logs(cmd.Context(), clients.LogsOptions{
			Application: application,
			Component:   component,
			Follow:      follow,
			Container:   container})
	},
}

func init() {
	componentCmd.AddCommand(logsCmd)

	logsCmd.Flags().String("container", "", "specify the container from which logs should be streamed")
	logsCmd.Flags().BoolP("follow", "f", false, "specify that logs should be stream until the command is canceled")
}
