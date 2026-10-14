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

package common

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func Test_RedirectStdout(t *testing.T) {
	original := os.Stdout

	out, restore := RedirectStdout()

	// Restore the original stdout on failure, but only if the explicit restore
	// below did not already run, so restore is called exactly once.
	restored := false
	defer func() {
		if !restored {
			restore()
		}
	}()

	// The returned writer is the original stdout so the progress UI can keep
	// rendering to the real terminal.
	require.Same(t, original, out)

	// The process's global stdout is redirected away from the terminal so stray
	// writes during installation are discarded instead of corrupting the UI.
	require.NotSame(t, original, os.Stdout)

	// Restoring puts the original stdout back in place.
	restore()
	restored = true
	require.Same(t, original, os.Stdout)
}

func Test_ProgressModel_Update_CtrlC(t *testing.T) {
	model := NewProgressModel(DisplayOptions{})

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	// Ctrl+C marks the model as interrupted so the caller can abort rad init.
	pm, ok := updated.(*ProgressModel)
	require.True(t, ok)
	require.True(t, pm.Interrupted)

	// The returned command quits the Bubble Tea program.
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, cmd())
}

// renderedText strips styling and trailing padding from a rendered view so it can be compared as plain text.
func renderedText(view tea.View) string {
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func Test_SummaryModel_View_ConfigFiles(t *testing.T) {
	t.Run("lists config files under the configuration heading", func(t *testing.T) {
		model := NewSummaryModel(DisplayOptions{
			ConfigFiles: []string{"/home/user/my-project/bicepconfig.json"},
		})

		text := renderedText(model.View())

		require.Contains(t, text, SummaryConfigurationHeadingIcon+"Update local configuration\n"+
			"   - /home/user/my-project/bicepconfig.json\n"+
			"\n(press enter to confirm or esc to restart)")
	})

	t.Run("lists no config files when empty", func(t *testing.T) {
		model := NewSummaryModel(DisplayOptions{})

		text := renderedText(model.View())

		require.Contains(t, text, SummaryConfigurationHeadingIcon+"Update local configuration\n"+
			"\n(press enter to confirm or esc to restart)")
	})
}

func Test_ProgressModel_View_ConfigFiles(t *testing.T) {
	complete := ProgressMsg{InstallComplete: true, EnvironmentComplete: true, ApplicationComplete: true, ConfigComplete: true}

	t.Run("lists config files under the configuration heading", func(t *testing.T) {
		model := NewProgressModel(DisplayOptions{
			ConfigFiles: []string{"/home/user/my-project/bicepconfig.json"},
		}).(*ProgressModel)
		model.Progress = complete

		text := renderedText(model.View())

		require.Contains(t, text, ProgressStepCompleteIcon+"Update local configuration\n"+
			"   - /home/user/my-project/bicepconfig.json\n"+
			"\nInitialization complete!")
	})

	t.Run("lists no config files when empty", func(t *testing.T) {
		model := NewProgressModel(DisplayOptions{}).(*ProgressModel)
		model.Progress = complete

		text := renderedText(model.View())

		require.Contains(t, text, ProgressStepCompleteIcon+"Update local configuration\n"+
			"\nInitialization complete!")
	})
}
