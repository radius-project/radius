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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/statearchive/factory"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_VersionWithoutArchiveConfiguration(t *testing.T) {
	const childEnv = "RADIUS_TEST_VERSION_WITHOUT_ARCHIVE"
	if os.Getenv(childEnv) == "1" {
		configPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}"), 0o600))
		RootCmd.SetArgs([]string{"version", "--cli", "--output", "json", "--config", configPath})
		require.NoError(t, RootCmd.ExecuteContext(t.Context()))
		return
	}

	executable, err := os.Executable()
	require.NoError(t, err)
	for _, backend := range []string{"", "oci", "git", "unknown"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv(childEnv, "1")
			t.Setenv(factory.BackendEnvVar, backend)
			t.Setenv(factory.StateRegistryEnvVar, "")
			t.Setenv(factory.GraphRegistryEnvVar, "")
			// A new process exercises root initialization with the environment
			// already set, rather than reusing this test process's global command.
			cmd := exec.CommandContext(t.Context(), executable, "-test.run=^Test_VersionWithoutArchiveConfiguration$")
			result, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", result)
			require.Contains(t, string(result), `"version":`)
			require.Contains(t, string(result), `"bicep":`)
		})
	}
}

func Test_HandlePanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("handlePanic should recover and not propagate panic")
		}
	}()

	func() {
		defer handlePanic()
		panic("test panic")
	}()
}

func Test_prettyPrintRPError(t *testing.T) {
	err := fmt.Errorf("test error message")
	result := prettyPrintRPError(err)
	require.Contains(t, result, "test error")
}

func Test_prettyPrintJSON(t *testing.T) {
	t.Run("formats JSON correctly", func(t *testing.T) {
		obj := map[string]string{"key": "value"}
		result, err := prettyPrintJSON(obj)
		require.NoError(t, err)
		require.Contains(t, result, "key")
		require.Contains(t, result, "value")
		require.Contains(t, result, "\n")
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		invalidObj := make(chan int)
		_, err := prettyPrintJSON(invalidObj)
		require.Error(t, err)
	})

	t.Run("formats complex objects", func(t *testing.T) {
		obj := map[string]any{
			"nested": map[string]string{"inner": "value"},
			"array":  []string{"a", "b", "c"},
		}
		result, err := prettyPrintJSON(obj)
		require.NoError(t, err)
		require.Contains(t, result, "nested")
		require.Contains(t, result, "inner")
		require.Contains(t, result, "array")
	})
}

func Test_wirePreviewSubcommand(t *testing.T) {
	t.Run("routes to legacy runner when --preview is not set", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		legacyCmd.SetArgs([]string{})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})

	t.Run("routes to preview runner when --preview is set", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		legacyCmd.SetArgs([]string{"--preview"})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("routes to preview runner when RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		t.Setenv("RADIUS_PREVIEW", "true")
		legacyCmd.SetArgs([]string{})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("routes to preview runner when RADIUS_PREVIEW=True (case-insensitive)", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		t.Setenv("RADIUS_PREVIEW", "True")
		legacyCmd.SetArgs([]string{})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("uses --preview=false to override RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		t.Setenv("RADIUS_PREVIEW", "true")
		legacyCmd.SetArgs([]string{"--preview=false"})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})

	t.Run("routes to legacy runner when RADIUS_PREVIEW is not true", func(t *testing.T) {
		legacyCalled := false
		previewCalled := false

		legacyCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { legacyCalled = true; return nil },
		}
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { previewCalled = true; return nil },
		}

		wirePreviewSubcommand(legacyCmd, previewCmd)

		t.Setenv("RADIUS_PREVIEW", "false")
		legacyCmd.SetArgs([]string{})
		err := legacyCmd.Execute()
		require.NoError(t, err)
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})
}

func Test_withPreviewEnvVarNote(t *testing.T) {
	t.Run("appends the RADIUS_PREVIEW note when absent", func(t *testing.T) {
		got := withPreviewEnvVarNote("Use the Radius.Core preview implementation for environment update")
		require.Equal(t, "Use the Radius.Core preview implementation for environment update (can also be set via RADIUS_PREVIEW=true)", got)
	})

	t.Run("does not double-append when the note is already present", func(t *testing.T) {
		usage := "Use the Radius.Core preview implementation (can also be set via RADIUS_PREVIEW=true)"
		require.Equal(t, usage, withPreviewEnvVarNote(usage))
	})

}

// Test_EnvCommands_RegisteredWithoutPreview checks that each `rad env` subcommand has no --preview
// flag and defines every flag its runner reads.
func Test_AppCommands_RegisteredWithoutPreview(t *testing.T) {
	testcases := []struct {
		command string
		flags   []string
	}{
		{command: "delete", flags: []string{"workspace", "group", "application", "yes", "force"}},
		{command: "graph", flags: []string{"workspace", "group", "application", "output", "include-icons"}},
		{command: "list", flags: []string{"workspace", "group", "output"}},
		{command: "show", flags: []string{"workspace", "group", "application", "output"}},
		{command: "status", flags: []string{"workspace", "group", "application", "output"}},
	}

	for _, tc := range testcases {
		t.Run(tc.command, func(t *testing.T) {
			cmd, _, err := RootCmd.Find([]string{"app", tc.command})
			require.NoError(t, err)
			require.Equal(t, tc.command, cmd.Name())

			require.Nil(t, cmd.Flags().Lookup("preview"), "rad app %s must not expose --preview", tc.command)
			for _, flag := range tc.flags {
				require.NotNilf(t, cmd.Flags().Lookup(flag), "rad app %s must expose --%s", tc.command, flag)
			}
		})
	}
}

func Test_EnvCommands_RegisteredWithoutPreview(t *testing.T) {
	testcases := []struct {
		command string
		flags   []string
	}{
		{command: "create", flags: []string{"workspace", "group", "kubernetes-namespace", "namespace", "recipe-packs", "recipe-pack-group", "azure-subscription-id", "azure-resource-group", "aws-region", "aws-account-id"}},
		{command: "delete", flags: []string{"workspace", "group", "environment", "yes", "force", "output"}},
		{command: "list", flags: []string{"workspace", "group", "output"}},
		{command: "show", flags: []string{"workspace", "group", "environment", "output"}},
		{command: "switch", flags: []string{"workspace", "environment"}},
		{command: "update", flags: []string{"workspace", "group", "environment", "kubernetes-namespace", "namespace", "clear-kubernetes", "recipe-packs", "recipe-pack-group", "azure-subscription-id", "azure-resource-group", "clear-azure", "aws-region", "aws-account-id", "clear-aws"}},
	}

	for _, tc := range testcases {
		t.Run(tc.command, func(t *testing.T) {
			cmd, _, err := RootCmd.Find([]string{"env", tc.command})
			require.NoError(t, err)
			require.Equal(t, tc.command, cmd.Name())

			require.Nil(t, cmd.Flags().Lookup("preview"), "rad env %s must not expose --preview", tc.command)
			for _, flag := range tc.flags {
				require.NotNilf(t, cmd.Flags().Lookup(flag), "rad env %s must expose --%s", tc.command, flag)
			}
		})
	}
}

// Test_InitAndWorkspaceCreate_RegisteredWithoutPreview checks that `rad init` and
// `rad workspace create` have no --preview flag and define every flag their runners read.
func Test_InitAndWorkspaceCreate_RegisteredWithoutPreview(t *testing.T) {
	testcases := []struct {
		args  []string
		name  string
		flags []string
	}{
		{args: []string{"init"}, name: "initialize", flags: []string{"full", "set", "set-file", "output"}},
		{args: []string{"workspace", "create"}, name: "create", flags: []string{"workspace", "group", "environment", "force", "context"}},
	}

	for _, tc := range testcases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd, _, err := RootCmd.Find(tc.args)
			require.NoError(t, err)
			require.Equal(t, tc.name, cmd.Name())

			require.Nil(t, cmd.Flags().Lookup("preview"), "rad %s must not expose --preview", strings.Join(tc.args, " "))
			for _, flag := range tc.flags {
				require.NotNilf(t, cmd.Flags().Lookup(flag), "rad %s must expose --%s", strings.Join(tc.args, " "), flag)
			}
		})
	}
}

func Test_ResourceList_ExposesPreviewFlag(t *testing.T) {
	listCmd, _, err := RootCmd.Find([]string{"resource", "list"})
	require.NoError(t, err)
	require.Equal(t, "list", listCmd.Name())
	require.NotNil(t, listCmd.Flags().Lookup("preview"), "rad resource list must expose --preview")
}

// Test_ResourceExposeAndLogs_RegisteredWithoutPreview checks that `rad resource expose` and
// `rad resource logs` have no --preview flag and define every flag their RunE reads.
func Test_ResourceExposeAndLogs_RegisteredWithoutPreview(t *testing.T) {
	testcases := []struct {
		command string
		flags   []string
	}{
		{command: "expose", flags: []string{"workspace", "group", "application", "type", "resource", "port", "remote-port", "replica"}},
		{command: "logs", flags: []string{"workspace", "group", "application", "follow", "container"}},
	}

	for _, tc := range testcases {
		t.Run(tc.command, func(t *testing.T) {
			cmd, _, err := RootCmd.Find([]string{"resource", tc.command})
			require.NoError(t, err)
			require.Equal(t, tc.command, cmd.Name())

			require.Nil(t, cmd.Flag("preview"), "rad resource %s must not expose --preview", tc.command)
			for _, flag := range tc.flags {
				require.NotNilf(t, cmd.Flag(flag), "rad resource %s must expose --%s", tc.command, flag)
			}
		})
	}
}
