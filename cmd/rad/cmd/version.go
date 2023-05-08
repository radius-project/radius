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
	"io"

	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the versions of the rad cli",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := cmd.Flags().GetBool("cli")
		if err != nil {
			return err
		}

		if !cli {
			outFormat, _ := cmd.Flags().GetString("output")
			writeVersionString(outFormat, cmd.OutOrStdout())
		} else {
			output.LogInfo(version.Version())
		}
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
			Heading:  "RELEASE",
			JSONPath: "{ .Release }",
		},
		{
			Heading:  "VERSION",
			JSONPath: "{ .Version }",
		},
		{
			Heading:  "BICEP",
			JSONPath: "{ .Bicep }",
		},
		{
			Heading:  "COMMIT",
			JSONPath: "{ .Commit }",
		},
	}})
}

func init() {
	RootCmd.AddCommand(versionCmd)
	versionCmd.Flags().Bool("cli", false, "Use this flag to only show the rad CLI version")
}
