// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource

import (
	"github.com/project-radius/radius/pkg/cli/cmd/resource/show"
	"github.com/project-radius/radius/pkg/cli/cmd/utils"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources",
		Long:  `Manage resources`,
	}
	cmd.PersistentFlags().StringP("application", "a", "", "The application name")
	cmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
	cmd.AddCommand(show.NewCommand(connections.DefaultFactory, utils.NewConfigHolder()))
	return cmd
}
