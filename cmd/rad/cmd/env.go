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
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(envCmd)
	envCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	envCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewEnvironmentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Manage Radius environments",
		Long: `Manage Radius environments
Radius environments are prepared “landing zones” for Radius applications. Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.`,
	}
}
