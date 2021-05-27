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

	"github.com/Azure/radius/pkg/workloads"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
		env, err := requireEnvironment(cmd)
		if err != nil {
			return err
		}

		application, err := requireApplication(cmd, env)
		if err != nil {
			return err
		}

		component, err := requireComponent(cmd, args)
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

		config, err := getMonitoringCredentials(cmd.Context(), *env)
		if err != nil {
			return err
		}

		client, err := k8s.NewForConfig(config)
		if err != nil {
			return err
		}

		replica, err := getRunningReplica(cmd.Context(), client, application, component)
		if err != nil {
			return err
		}

		if container == "" {
			// We don't really expect this to fail, but let's do something reasonable if it does...
			container = getAppContainerName(replica)
			if container == "" {
				return fmt.Errorf("failed to find the default container for component '%s'. use '--container <name>' to specify the name", component)
			}
		}

		stream, err := streamLogs(cmd.Context(), config, client, replica, container, follow)
		if err != nil {
			return fmt.Errorf("failed to open log stream to %s: %w", component, err)
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

		// Unreachable
	},
}

func init() {
	componentCmd.AddCommand(logsCmd)

	logsCmd.Flags().String("container", "", "specify the container from which logs should be streamed")
	logsCmd.Flags().BoolP("follow", "f", false, "specify that logs should be stream until the command is canceled")
}

func getAppContainerName(replica *corev1.Pod) string {
	// The container name will be the component name
	component := replica.Labels[workloads.LabelRadiusComponent]
	return component
}

func streamLogs(ctx context.Context, config *rest.Config, client *k8s.Clientset, replica *corev1.Pod, container string, follow bool) (io.ReadCloser, error) {
	options := &corev1.PodLogOptions{
		Container: container,
		Follow:    follow,
	}

	request := client.CoreV1().Pods(replica.Namespace).GetLogs(replica.Name, options)
	return request.Stream(ctx)
}
