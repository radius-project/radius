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

// Test_wirePreviewSubcommandPreviewBase_PreviewOnlyFlags verifies that preview-only flags are
// rejected when preview mode is off, instead of silently routing to the legacy runner (which
// would ignore them).
func Test_wirePreviewSubcommandPreviewBase_PreviewOnlyFlags(t *testing.T) {
	newCmd := func(legacyCalled, previewCalled *bool) *cobra.Command {
		previewCmd := &cobra.Command{
			Use:  "test",
			RunE: func(cmd *cobra.Command, args []string) error { *previewCalled = true; return nil },
		}
		previewCmd.Flags().StringSlice("recipe-packs", nil, "preview-only flag")
		legacyRunE := func(cmd *cobra.Command, args []string) error { *legacyCalled = true; return nil }
		wirePreviewSubcommandPreviewBase(previewCmd, legacyRunE, "Use the preview implementation.", "recipe-packs")
		return previewCmd
	}

	t.Run("rejects a preview-only flag when preview is off", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmd(&legacyCalled, &previewCalled)

		cmd.SetArgs([]string{"--recipe-packs", "p1"})
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires preview mode")
		require.False(t, legacyCalled, "legacy runner must not run when a preview-only flag is set")
		require.False(t, previewCalled, "preview runner must not run when preview is off")
	})

	t.Run("allows a preview-only flag when --preview is set", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmd(&legacyCalled, &previewCalled)

		cmd.SetArgs([]string{"--preview", "--recipe-packs", "p1"})
		require.NoError(t, cmd.Execute())
		require.True(t, previewCalled, "preview runner should have been called")
		require.False(t, legacyCalled, "legacy runner should not have been called")
	})

	t.Run("routes to legacy runner when no preview-only flag is set", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmd(&legacyCalled, &previewCalled)

		cmd.SetArgs([]string{})
		require.NoError(t, cmd.Execute())
		require.True(t, legacyCalled, "legacy runner should have been called")
		require.False(t, previewCalled, "preview runner should not have been called")
	})

	t.Run("allows a preview-only flag when RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmd(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "true")
		cmd.SetArgs([]string{"--recipe-packs", "p1"})
		require.NoError(t, cmd.Execute())
		require.True(t, previewCalled, "preview runner should have been called")
		require.False(t, legacyCalled, "legacy runner should not have been called")
	})

	t.Run("rejects a preview-only flag when --preview=false overrides RADIUS_PREVIEW=true", func(t *testing.T) {
		legacyCalled, previewCalled := false, false
		cmd := newCmd(&legacyCalled, &previewCalled)

		t.Setenv("RADIUS_PREVIEW", "true")
		cmd.SetArgs([]string{"--preview=false", "--recipe-packs", "p1"})
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires preview mode")
		require.False(t, legacyCalled, "legacy runner must not run when a preview-only flag is set")
		require.False(t, previewCalled, "preview runner must not run when preview is off")
	})
}

// Test_wirePreviewSubcommandPreviewBase_UnknownPreviewOnlyFlagPanics verifies that wiring a
// preview-only flag name that is not defined on the command fails loudly, rather than silently
// disabling the guard.
func Test_wirePreviewSubcommandPreviewBase_UnknownPreviewOnlyFlagPanics(t *testing.T) {
	previewCmd := &cobra.Command{
		Use:  "test",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	legacyRunE := func(cmd *cobra.Command, args []string) error { return nil }

	require.PanicsWithValue(t,
		`wirePreviewSubcommandPreviewBase: preview-only flag "does-not-exist" is not defined on command "test"`,
		func() {
			wirePreviewSubcommandPreviewBase(previewCmd, legacyRunE, "Use the preview implementation.", "does-not-exist")
		},
	)
}

// Test_EnvCreate_ExposesPreviewRecipePacksFlag guards the command-tree wiring for
// `rad env create`. The --recipe-packs flag is defined only on the preview
// implementation, so env create must be wired with the preview command as the
// base (wirePreviewSubcommandPreviewBase). If it regresses to a legacy-base
// wiring, the flag becomes unreachable and `rad env create --preview
// --recipe-packs` fails with "flag accessed but not defined". RootCmd is fully
// assembled at package init(), so this asserts the real wiring, not a helper.
func Test_EnvCreate_ExposesPreviewRecipePacksFlag(t *testing.T) {
	createCmd, _, err := RootCmd.Find([]string{"env", "create"})
	require.NoError(t, err)
	require.Equal(t, "create", createCmd.Name())

	// RootCmd is a shared global; restore the flags this test mutates so it does not
	// leak --preview/--recipe-packs state into other tests.
	t.Cleanup(func() {
		_ = createCmd.Flags().Set("preview", createCmd.Flags().Lookup("preview").DefValue)
		_ = createCmd.Flags().Set("recipe-packs", createCmd.Flags().Lookup("recipe-packs").DefValue)
	})

	require.NotNil(t, createCmd.Flags().Lookup("recipe-packs"),
		"rad env create must expose --recipe-packs (regression: wrong preview wiring)")
	require.NotNil(t, createCmd.Flags().Lookup("preview"),
		"rad env create must expose --preview")

	require.NoError(t, createCmd.ParseFlags([]string{"--preview", "--recipe-packs", "p1,p2"}),
		"rad env create must accept --preview together with --recipe-packs")
}

func Test_ResourceList_ExposesPreviewFlag(t *testing.T) {
	listCmd, _, err := RootCmd.Find([]string{"resource", "list"})
	require.NoError(t, err)
	require.Equal(t, "list", listCmd.Name())
	require.NotNil(t, listCmd.Flags().Lookup("preview"), "rad resource list must expose --preview")
}

// Test_EnvPreviewOnlyFlagsRejectedWithoutPreview drives the real, fully-assembled command tree
// and asserts that each preview-only flag is rejected when preview mode is off. This guards
// against a preview-only flag being added to a command without also being registered in the
// wirePreviewSubcommandPreviewBase guard, which would let it silently fall through to the legacy
// runner. env create's only preview-only flag is --recipe-packs; env update additionally has
// --clear-kubernetes.
func Test_EnvPreviewOnlyFlagsRejectedWithoutPreview(t *testing.T) {
	testcases := []struct {
		command string
		flag    string
		value   string
	}{
		{command: "create", flag: "recipe-packs", value: "p1"},
		{command: "update", flag: "recipe-packs", value: "p1"},
		{command: "update", flag: "clear-kubernetes", value: "true"},
	}

	for _, tc := range testcases {
		t.Run(tc.command+" --"+tc.flag, func(t *testing.T) {
			cmd, _, err := RootCmd.Find([]string{"env", tc.command})
			require.NoError(t, err)

			// RootCmd is a shared global; a prior test may have left --preview set.
			// Force preview off so this test deterministically exercises the guard.
			require.NoError(t, cmd.Flags().Set("preview", "false"))
			t.Cleanup(func() { _ = cmd.Flags().Set("preview", cmd.Flags().Lookup("preview").DefValue) })

			require.NoError(t, cmd.Flags().Set(tc.flag, tc.value))
			t.Cleanup(func() { _ = cmd.Flags().Set(tc.flag, cmd.Flags().Lookup(tc.flag).DefValue) })

			err = cmd.RunE(cmd, []string{"someenv"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "requires preview mode",
				"--%s must be rejected without --preview", tc.flag)
		})
	}
}
