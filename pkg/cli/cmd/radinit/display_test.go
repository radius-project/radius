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
	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Set this to a big value when debugging.
	waitTimeout = 5 * time.Second
)

func Test_summaryModel(t *testing.T) {
	waitForRender := func(t *testing.T, reader io.Reader) string {
		normalized := ""
		teatest.WaitFor(t, reader, func(bts []byte) bool {
			normalized = stripansi.Strip(strings.ReplaceAll(string(bts), "\r\n", "\n"))
			return strings.Contains(normalized, strings.Trim(common.SummaryFooter, "\n"))
		}, teatest.WithDuration(waitTimeout))

		return normalized
	}

	resultTest := func(t *testing.T, expected common.SummaryResult, key tea.KeyType) {
		options := initOptions{}
		model := &common.SummaryModel{
			Options: toDisplayOptions(&options),
		}
		tm := teatest.NewTestModel(t, model)

		waitForRender(t, tm.Output())

		// Press the given key
		tm.Send(tea.KeyMsg{
			Type: key,
		})

		if err := tm.Quit(); err != nil {
			t.Fatal(err)
		}

		// FinalModel only returns once the program has finished running or when it times out.
		// Please see: https://github.com/charmbracelet/x/blob/20117e9c8cd5ad229645f1bca3422b7e4110c96c/exp/teatest/teatest.go#L220.
		// That is why we call tm.Quit() before tm.FinalModel().
		model = tm.FinalModel(t).(*common.SummaryModel)
		require.Equal(t, expected, model.Result)
	}

	t.Run("Result: Confirm", func(t *testing.T) {
		resultTest(t, common.ResultConfirmed, tea.KeyEnter)
	})

	t.Run("Result: Cancel", func(t *testing.T) {
		resultTest(t, common.ResultCanceled, tea.KeyEscape)
	})

	t.Run("Result: Quit", func(t *testing.T) {
		resultTest(t, common.ResultQuit, tea.KeyCtrlC)
	})

	viewTest := func(t *testing.T, options initOptions, expected string) {
		model := &common.SummaryModel{
			Options: toDisplayOptions(&options),
		}
		tm := teatest.NewTestModel(t, model)

		output := waitForRender(t, tm.Output())
		assert.Equal(t, expected, output)

		// Press ENTER
		tm.Send(tea.KeyMsg{
			Type: tea.KeyEnter,
		})

		if err := tm.Quit(); err != nil {
			t.Fatal(err)
		}

		// FinalModel only returns once the program has finished running or when it times out.
		// Please see: https://github.com/charmbracelet/x/blob/20117e9c8cd5ad229645f1bca3422b7e4110c96c/exp/teatest/teatest.go#L220.
		// That is why we call tm.Quit() before tm.FinalModel().
		model = tm.FinalModel(t).(*common.SummaryModel)
		assert.Equal(t, common.SummaryResult(common.ResultConfirmed), model.Result)
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

		expected := "\rYou've selected the following:\n" +
			"\n" +
			"🔧 Use existing Radius test-version install on test-context\n" +
			"🌏 Use existing environment test-environment\n" +
			"📋 Update local configuration\n" +
			"\n" +
			"(press enter to confirm or esc to restart)\n\r"

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

		expected := "\rYou've selected the following:\n" +
			"\n" +
			"🔧 Install Radius test-version\n" +
			"   - Kubernetes cluster: test-context\n" +
			"   - Kubernetes namespace: \n" +
			"🌏 Use existing environment test-environment\n" +
			"📋 Update local configuration\n" +
			"\n" +
			"(press enter to confirm or esc to restart)\n\r"

		viewTest(t, options, expected)
	})
}
