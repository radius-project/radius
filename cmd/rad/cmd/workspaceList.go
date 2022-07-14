// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"sort"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local workspaces",
	Long:  `List local workspaces`,
	RunE:  listWorkspaces,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
}

func listWorkspaces(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	section, err := cli.ReadWorkspaceSection(config)
	if err != nil {
		return err
	}

	// Put in alphabetical order in a slice
	names := []string{}
	for name := range section.Items {
		names = append(names, name)
	}

	sort.Strings(names)

	items := []workspaces.Workspace{}
	for _, name := range names {
		items = append(items, section.Items[name])
	}

	err = output.Write(format, items, cmd.OutOrStdout(), objectformats.GetWorkspaceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
