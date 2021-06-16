// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/clients"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [component]",
	Short: "Read logs from a running container component",
	Long: `Reads logs from a running component. Currently only supports the component kind 'radius.dev/Container'.
This command allows you to access logs of a deployed application and output those logs to the local console.

'rad component logs' will output all currently available logs for the component and then exit.

'rad component logs' will output logs from the component's primary container. In scenarios like Dapr where multiple containers are in use, the '--container \<name\>' option can specify the desired container.

Specify the '--follow' option to stream additional logs as they are emitted by the component. When following, press CTRL+C to exit the command and terminate the stream.`,
	Example: `# read logs from the 'webapp' component of the current default app
rad component logs webapp

# read logs from the 'orders' component of the 'icecream-store' application
rad component logs orders --application icecream-store

# stream logs from the 'orders' component of the 'icecream-store' application
rad component logs orders --application icecream-store --follow

# read logs from the 'daprd' sidecar container of the 'orders' component of the 'icecream-store' application
rad component logs orders --application icecream-store --container daprd`,
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

		client, err := environments.CreateDiagnosticsClient(cmd.Context(), env)
		if err != nil {
			return err
		}

		stream, err := client.Logs(cmd.Context(), clients.LogsOptions{
			Application: application,
			Component:   component,
			Follow:      follow,
			Container:   container})
		if err != nil {
			return err
		}

		defer stream.Close()

		// We can keep reading this until cancellation occurs.
		if follow {
			// Sending to stderr so it doesn't interfere with parsing
			fmt.Fprintf(os.Stderr, "Streaming logs from component %s. Press CTRL+C to exit...\n", component)
		}

		hasLogs := false
		reader := bufio.NewReader(stream)
		for {
			line, prefix, err := reader.ReadLine()
			if err == context.Canceled {
				// CTRL+C => done
				return nil
			} else if err == io.EOF {
				// End of stream
				//
				// Output a status message to stderr if there were no logs for non-streaming
				// so an interactive user gets *some* feedback.
				if !follow && !hasLogs {
					fmt.Fprintln(os.Stderr, "Component's log is currently empty.")
				}

				return nil
			} else if err != nil {
				return fmt.Errorf("failed to read log stream %T: %w", err, err)
			}

			hasLogs = true

			// Handle the case where a partial line is returned
			if prefix {
				fmt.Print(string(line))
				continue
			}

			fmt.Println(string(line))
		}

	},
}

func init() {
	componentCmd.AddCommand(logsCmd)

	logsCmd.Flags().String("container", "", "specify the container from which logs should be streamed")
	logsCmd.Flags().BoolP("follow", "f", false, "specify that logs should be stream until the command is canceled")
}
