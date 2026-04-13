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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/gitstate"
	"github.com/radius-project/radius/pkg/cli/graph"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Application graph commands",
	Long:  "Commands for building and managing Radius application graph artifacts.",
}

var graphBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a static application graph from a Bicep file",
	Long: `Build a static application graph JSON artifact from a Bicep application definition.

This command compiles the Bicep file to ARM JSON, parses resources and connections,
maps source line numbers, computes diff hashes, and emits a static graph artifact
suitable for consumption by the Radius browser extension.

By default, the artifact is written to a local file. Use --orphan-branch to commit
the artifact to a git orphan branch instead, stored under {source-branch}/app.json.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bicepFile, _ := cmd.Flags().GetString("bicep")
		outputPath, _ := cmd.Flags().GetString("output")
		orphanBranch, _ := cmd.Flags().GetString("orphan-branch")
		sourceBranch, _ := cmd.Flags().GetString("source-branch")

		// Compile Bicep to ARM JSON using bicep CLI.
		armJSONPath, err := compileBicep(bicepFile)
		if err != nil {
			return fmt.Errorf("compiling Bicep file: %w", err)
		}
		defer os.Remove(armJSONPath)

		// Build the static graph.
		artifact, err := graph.BuildStaticGraph(armJSONPath, bicepFile)
		if err != nil {
			return fmt.Errorf("building static graph: %w", err)
		}

		// Marshal to JSON.
		data, err := json.MarshalIndent(artifact, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling graph artifact: %w", err)
		}

		// If --orphan-branch is set, commit to the orphan branch.
		if orphanBranch != "" {
			if sourceBranch == "" {
				return fmt.Errorf("--source-branch is required when using --orphan-branch")
			}
			return commitToOrphanBranch(cmd, data, orphanBranch, sourceBranch)
		}

		// Otherwise write to the local filesystem.
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("writing graph artifact: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Static graph artifact written to %s\n", outputPath)
		return nil
	},
}

// commitToOrphanBranch writes the graph artifact to a git orphan branch at
// {sourceBranch}/app.json using the gitstate package.
func commitToOrphanBranch(cmd *cobra.Command, data []byte, orphanBranch, sourceBranch string) error {
	ctx := context.Background()

	wt, err := gitstate.OpenOrCreate(ctx, orphanBranch)
	if err != nil {
		return fmt.Errorf("opening orphan branch %q: %w", orphanBranch, err)
	}
	defer wt.Remove(ctx)

	artifactRelPath := filepath.Join(sourceBranch, "app.json")
	if err := wt.WriteFile(artifactRelPath, data); err != nil {
		return fmt.Errorf("writing artifact to orphan branch: %w", err)
	}

	commitMsg := fmt.Sprintf("chore: update app graph for %s [skip ci]", sourceBranch)
	if err := wt.CommitAndPush(ctx, commitMsg); err != nil {
		return fmt.Errorf("committing to orphan branch: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Static graph artifact committed to %s branch at %s\n", orphanBranch, artifactRelPath)
	return nil
}

func init() {
	RootCmd.AddCommand(graphCmd)

	graphBuildCmd.Flags().String("bicep", "app.bicep", "Path to the Bicep application definition file")
	graphBuildCmd.Flags().String("output", ".radius/static/app.json", "Path for the output graph artifact (local file mode)")
	graphBuildCmd.Flags().String("orphan-branch", "", "Commit the artifact to this git orphan branch instead of writing a local file")
	graphBuildCmd.Flags().String("source-branch", "", "Source branch name used as the directory prefix on the orphan branch (required with --orphan-branch)")
	graphCmd.AddCommand(graphBuildCmd)
}

// compileBicep runs `bicep build` on the given file and returns the path to the compiled ARM JSON.
func compileBicep(bicepFile string) (string, error) {
	// Create a temp file for the ARM JSON output.
	tmpFile, err := os.CreateTemp("", "radius-graph-*.json")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Try 'bicep build' first, then fall back to 'az bicep build'.
	cmd := exec.Command("bicep", "build", bicepFile, "--outfile", tmpPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try az bicep as fallback.
		azCmd := exec.Command("az", "bicep", "build", "--file", bicepFile, "--outfile", tmpPath)
		if azOutput, azErr := azCmd.CombinedOutput(); azErr != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("bicep build failed: %s\naz bicep build failed: %s", string(output), string(azOutput))
		}
	}

	return tmpPath, nil
}
