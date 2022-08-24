// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource

import (
	"github.com/project-radius/radius/pkg/cli/cmd/resource/show"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

func NewCommand(framework framework.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources",
		Long:  `Manage resources`,
	}
	cmd.PersistentFlags().StringP("application", "a", "", "The application name")
	cmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")

	showCmd, _ := show.NewCommand(framework)
	cmd.AddCommand(showCmd)
	return cmd
}
