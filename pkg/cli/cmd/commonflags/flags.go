// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package commonflags

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

func AddOutputFlag(cmd *cobra.Command) {
	description := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	cmd.Flags().StringP("output", "o", output.DefaultFormat, description)
}

func AddWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
}

func AddResourceGroupFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("group", "g", "", "The resource group name")
}

func AddApplicationNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("application", "a", "", "The application name")
}

func AddConfirmationFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("yes", "y", false, "The confirmation flag")
}

func AddEnvironmentNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("environment", "e", "", "The environment name")
}

func AddNamespaceFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("namespace", "n", "", "The Kubernetes namespace")
}

func AddParameterFlag(cmd *cobra.Command) {
	cmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")
}
