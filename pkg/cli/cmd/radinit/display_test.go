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

package radinit

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acarl005/stripansi"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Set this to a big value when debugging.
	waitTimeout = 1 * time.Second
)

// NOTE: I tried my best to write a test for the progress model. It was possible to write a good test, but
// I ran into bugs with https://github.com/charmbracelet/x/tree/main/exp/teatest truncating the output.
//
// We can try again when the test framework is more mature.

func Test_summaryModel(t *testing.T) {
	waitForRender := func(t *testing.T, reader io.Reader) string {
		normalized := ""
		teatest.WaitFor(t, reader, func(bts []byte) bool {
			normalized = stripansi.Strip(strings.ReplaceAll(string(bts), "\r\n", "\n"))
			return strings.Contains(normalized, strings.Trim(summaryFooter, "\n"))
		}, teatest.WithDuration(waitTimeout))

		return normalized
	}
	waitForEmpty := func(t *testing.T, reader io.Reader) string {
		normalized := ""
		teatest.WaitFor(t, reader, func(bts []byte) bool {
			normalized = stripansi.Strip(strings.ReplaceAll(string(bts), "\r\n", "\n"))
			return !strings.Contains(normalized, strings.Trim(summaryFooter, "\n"))
		}, teatest.WithDuration(waitTimeout))

		return normalized
	}

	resultTest := func(t *testing.T, expected summaryResult, key tea.KeyType) {
		options := initOptions{}
		model := &summaryModel{
			options: options,
		}
		tm := teatest.NewTestModel(t, model)

		waitForRender(t, tm.Output())

		// Press ENTER
		tm.Send(tea.KeyMsg{Type: key})

		// Wait for final render and exit.
		tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
		waitForEmpty(t, tm.FinalOutput(t))
		model = tm.FinalModel(t).(*summaryModel)
		require.Equal(t, expected, model.result)
	}

	t.Run("Result: Confirm", func(t *testing.T) {
		resultTest(t, resultConfimed, tea.KeyEnter)
	})
	t.Run("Result: Cancel", func(t *testing.T) {
		resultTest(t, resultCanceled, tea.KeyEscape)
	})
	t.Run("Result: Quit", func(t *testing.T) {
		resultTest(t, resultQuit, tea.KeyCtrlC)
	})

	viewTest := func(t *testing.T, options initOptions, expected string) {
		model := &summaryModel{
			options: options,
		}
		tm := teatest.NewTestModel(t, model)

		output := waitForRender(t, tm.Output())
		assert.Equal(t, expected, output)

		// Press ENTER
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

		// Wait for final render and exit.
		tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
		waitForEmpty(t, tm.FinalOutput(t))
		model = tm.FinalModel(t).(*summaryModel)
		assert.Equal(t, summaryResult(resultConfimed), model.result)
	}

	t.Run("View: existing options", func(t *testing.T) {
		options := initOptions{
			Cluster: clusterOptions{
				Install: false,
				Context: "test-context",
				Version: "test-version",
			},
			Environment: environmentOptions{
				Create: false,
				Name:   "test-environment",
			},
		}

		expected := "You've selected the following:\n" +
			"\n" +
			"🔧 Use existing Radius test-version install on test-context\n" +
			"🌏 Use existing environment test-environment\n" +
			"📋 Update local configuration\n" +
			"\n" +
			"(press enter to confirm or esc to restart)\n"

		viewTest(t, options, expected)
	})

	t.Run("View: full options", func(t *testing.T) {
		options := initOptions{
			Cluster: clusterOptions{
				Install: true,
				Context: "test-context",
				Version: "test-version",
			},
			Environment: environmentOptions{
				Create: false,
				Name:   "test-environment",
			},
			CloudProviders: cloudProviderOptions{},
			Recipes:        recipePackOptions{},
			Application:    applicationOptions{},
		}

		expected := "You've selected the following:\n" +
			"\n" +
			"🔧 Install Radius test-version\n" +
			"   - Kubernetes cluster: test-context\n" +
			"   - Kubernetes namespace: \n" +
			"🌏 Use existing environment test-environment\n" +
			"📋 Update local configuration\n" +
			"\n" +
			"(press enter to confirm or esc to restart)\n"

		viewTest(t, options, expected)
	})
}
