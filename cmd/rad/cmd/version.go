// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

type displayVersion struct {
	Release string `json:"release"`
	Build   string `json:"version"`
	Bicep   string `json:"bicep"`
	Commit  string `json:"commit"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version numbers of rad components",
	Long:  `All software has versions. This is rad's.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outFormat, _ := cmd.Flags().GetString("output")
		println(getVersionString(outFormat))
		return nil
	},
}

func getVersionString(f string) string {
	v := displayVersion{
		Release: version.Release(),
		Build:   version.Version(),
		Bicep:   bicep.Version(),
		Commit:  version.Commit(),
	}
	switch f {
	case output.FormatJson:
		jsonStr, _ := json.MarshalIndent(v, "", "    ")
		return fmt.Sprintln(string(jsonStr))
	default:
		formatStr := "Release: %s \nVersion: %s\nBicep version: %s\nCommit: %s\n"
		return fmt.Sprintf(formatStr, v.Release, v.Build, v.Bicep, v.Commit)
	}
}

func init() {
	RootCmd.AddCommand(versionCmd)

}
