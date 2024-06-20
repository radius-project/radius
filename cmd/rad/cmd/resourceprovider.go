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
	RootCmd.AddCommand(resourceProviderCmd)
	resourceProviderCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewResourceProviderCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resourceprovider",
		Short: "Manage resource providers",
		Long:  `Manage resource providers`,
	}
}
