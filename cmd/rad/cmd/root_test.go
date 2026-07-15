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
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

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

	t.Run("preview-base wiring exposes a --preview flag mentioning RADIUS_PREVIEW", func(t *testing.T) {
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		wirePreviewSubcommandPreviewBase(previewCmd, func(cmd *cobra.Command, args []string) error { return nil }, "Use the preview implementation")

		flag := previewCmd.Flags().Lookup("preview")
		require.NotNil(t, flag)
		require.Contains(t, flag.Usage, "RADIUS_PREVIEW")
	})
}

func Test_wirePreviewSubcommandPreviewBase(t *testing.T) {
	// newCmds builds a preview command (the base) plus a legacy runner, wired together via
	// wirePreviewSubcommandPreviewBase. The returned pointers report which runner executed.
	newCmds := func(legacyCalled, previewCalled *bool) *cobra.Command {
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { *previewCalled = true; return nil },
		}
		legacyRunE := func(cmd *cobra.Command, args []string) error { *legacyCalled = true; return nil }
		wirePreviewSubcommandPreviewBase(previewCmd, legacyRunE, "Use the preview implementation.")
		return previewCmd
	}

	t.Run("routes to legacy runner when --preview is not set", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		cmd.SetArgs([]string{})
		require.NoError(t, cmd.Execute())
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})

	t.Run("routes to preview runner when --preview is set", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		cmd.SetArgs([]string{"--preview"})
		require.NoError(t, cmd.Execute())
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("routes to preview runner when RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "true")
		cmd.SetArgs([]string{})
		require.NoError(t, cmd.Execute())
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("routes to preview runner when RADIUS_PREVIEW=True (case-insensitive)", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "True")
		cmd.SetArgs([]string{})
		require.NoError(t, cmd.Execute())
		require.False(t, legacyCalled, "legacy runner should not have been called")
		require.True(t, previewCalled, "preview runner should have been called")
	})

	t.Run("uses --preview=false to override RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "true")
		cmd.SetArgs([]string{"--preview=false"})
		require.NoError(t, cmd.Execute())
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})

	t.Run("routes to legacy runner when RADIUS_PREVIEW is not true", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmds(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "false")
		cmd.SetArgs([]string{})
		require.NoError(t, cmd.Execute())
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})
}
