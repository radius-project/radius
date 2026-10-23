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
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Test_ResourceExposeAndLogs_RejectNonRadiusComputeContainers checks that `rad resource expose` and
// `rad resource logs` reject any resource type other than Radius.Compute/containers before creating a client.
func Test_ResourceExposeAndLogs_RejectNonRadiusComputeContainers(t *testing.T) {
	t.Parallel()

	commands := []struct {
		name string
		run  func(*cobra.Command, []string) error
	}{
		{name: "expose", run: exposeResource},
		{name: "logs", run: logsResource},
	}
	resourceTypes := []string{"Applications.Core/containers", "Radius.Compute/gateways"}

	for _, command := range commands {
		for _, resourceType := range resourceTypes {
			t.Run(command.name+"/"+resourceType, func(t *testing.T) {
				t.Parallel()

				cmd := &cobra.Command{}
				cmd.Flags().String("workspace", "", "")
				cmd.Flags().String("group", "", "")
				cmd.Flags().String("application", "", "")
				require.NoError(t, cmd.Flags().Set("group", "test-group"))
				require.NoError(t, cmd.Flags().Set("application", "demo"))
				cmd.SetContext(context.WithValue(t.Context(), framework.NewContextKey("config"), &framework.ConfigHolder{Config: viper.New()}))

				err := command.run(cmd, []string{resourceType, "web"})
				require.EqualError(t, err, "only Radius.Compute/containers is supported")
			})
		}
	}
}
