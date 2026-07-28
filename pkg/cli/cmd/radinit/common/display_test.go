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
	"testing"

	tea "charm.land/bubbletea/v2"
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
