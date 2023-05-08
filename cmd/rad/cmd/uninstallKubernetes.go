/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package cmd

import (
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/spf13/cobra"
)

var uninstallKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Uninstall Radius from Kubernetes Cluster",
	Long:  `Uninstall Radius from Kubernetes Cluster`,
	RunE:  uninstallKubernetes,
}

func init() {
	uninstallCmd.AddCommand(uninstallKubernetesCmd)

	uninstallKubernetesCmd.Flags().String("kubecontext", "", "the Kubernetes context to use, will use the default if unset")
}

func uninstallKubernetes(cmd *cobra.Command, args []string) error {
	kubeContext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	// It's OK for KubeContext to be blank.
	err = setup.Uninstall(cmd.Context(), kubeContext)
	if err != nil {
		return err
	}

	return nil
}
