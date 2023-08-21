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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/spf13/cobra"
)

var resourceLogsCmd = &cobra.Command{
	Use:   "logs [resource]",
	Short: "Read logs from a running containers resource",
	Long: `Reads logs from a running resource. Currently only supports the resource type 'Applications.Core/containers'.
This command allows you to access logs of a deployed application and output those logs to the local console.

'rad resource logs' will output all currently available logs for the resource and then exit.

'rad resource logs' will output logs from the resource's primary container. In scenarios like Dapr where multiple containers are in use, the '--container \<name\>' option can specify the desired container.

Specify the '--follow' option to stream additional logs as they are emitted by the resource. When following, press CTRL+C to exit the command and terminate the stream.`,
	Example: `# read logs from the 'webapp' resource of the current default app
rad resource logs containers webapp

# read logs from the 'orders' resource of the 'icecream-store' application
rad resource logs containers orders --application icecream-store

# stream logs from the 'orders' resource of the 'icecream-store' application
rad resource logs containers orders --application icecream-store --follow

# read logs from the 'daprd' sidecar container of the 'orders' resource of the 'icecream-store' application
rad resource logs containers orders --application icecream-store --container daprd`,
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

		application, err := cli.RequireApplication(cmd, *workspace)
		if err != nil {
			return err
		}

		resourceType, resourceName, err := cli.RequireResource(cmd, args)
		if err != nil {
			return err
		}
		if !strings.EqualFold(resourceType, ContainerType) {
			return fmt.Errorf("only %s is supported", ContainerType)
		}
		follow, err := cmd.Flags().GetBool("follow")
		if err != nil {
			return err
		}

		container, err := cmd.Flags().GetString("container")
		if err != nil {
			return err
		}

		var client clients.DiagnosticsClient
		client, err = connections.DefaultFactory.CreateDiagnosticsClient(cmd.Context(), *workspace)
		if err != nil {
			return err
		}

		streams, err := client.Logs(cmd.Context(), clients.LogsOptions{
			Application: application,
			Resource:    resourceName,
			Follow:      follow,
			Container:   container})
		if err != nil {
			return err
		}

		logErrors := make(chan error, len(streams))
		for _, logInfo := range streams {

			// We can keep reading this until cancellation occurs.
			if follow {
				// Sending to stderr so it doesn't interfere with parsing
				fmt.Fprintf(os.Stderr, "Streaming logs from replica %s for Container %s. Press CTRL+C to exit...\n", logInfo.Name, resourceName)
			}

			// Kick off go routine to read the logs from each stream.
			go captureLogs(logInfo, logErrors, follow)
		}

		for i := 0; i < len(streams); i++ {
			err := <-logErrors
			if err != nil {
				// TODO format
				fmt.Fprintln(os.Stderr, err)
			}
		}
		return nil
	},
}

func captureLogs(info clients.LogStream, logErrors chan<- error, follow bool) {
	stream := info.Stream
	defer stream.Close()

	name := info.Name
	hasLogs := false
	reader := bufio.NewReader(stream)
	startLine := true
	for {
		line, prefix, err := reader.ReadLine()
		if err == context.Canceled {
			// CTRL+C => done
			logErrors <- nil
			return
		} else if err == io.EOF {
			// End of stream
			//
			// Output a status message to stderr if there were no logs for non-streaming
			// so an interactive user gets *some* feedback.
			if !follow && !hasLogs {
				fmt.Fprintln(os.Stderr, "Containers's log is currently empty.")
			}
			logErrors <- nil
			return
		} else if err != nil {
			logErrors <- err
			return
		}

		hasLogs = true

		// Handle the case where a partial line is returned
		if prefix {
			if startLine {
				fmt.Print("[" + name + "] " + string(line))
			} else {
				fmt.Print(string(line))
			}
			startLine = false
			continue
		}

		if startLine {
			fmt.Println("[" + name + "] " + string(line))
		} else {
			fmt.Println(string(line))
		}
		startLine = true
	}
}

func init() {
	resourceLogsCmd.Flags().String("container", "", "specify the container from which logs should be streamed")
	resourceLogsCmd.Flags().BoolP("follow", "f", false, "specify that logs should be stream until the command is canceled")
	resourceLogsCmd.Flags().String("replica", "", "specify the replica to collect logs from")
	commonflags.AddResourceGroupFlag(resourceLogsCmd)
	resourceCmd.AddCommand(resourceLogsCmd)
}
