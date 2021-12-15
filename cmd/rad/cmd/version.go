// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"io"

	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the versions of the rad cli",
	RunE: func(cmd *cobra.Command, args []string) error {
		outFormat, _ := cmd.Flags().GetString("output")
		writeVersionString(outFormat, cmd.OutOrStdout())
		return nil
	},
}

func writeVersionString(format string, w io.Writer) {
	var displayVersion = struct {
		Release string `json:"release"`
		Version string `json:"version"`
		Bicep   string `json:"bicep"`
		Commit  string `json:"commit"`
	}{
		version.Release(),
		version.Version(),
		bicep.Version(),
		version.Commit(),
	}
	_ = output.Write(format, displayVersion, w, output.FormatterOptions{Columns: []output.Column{
		{
			Heading:  "Release",
			JSONPath: "{ .Release }",
		},
		{
			Heading:  "Version",
			JSONPath: "{ .Version }",
		},
		{
			Heading:  "Bicep",
			JSONPath: "{ .Bicep }",
		},
		{
			Heading:  "Commit",
			JSONPath: "{ .Commit }",
		},
	}})
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
