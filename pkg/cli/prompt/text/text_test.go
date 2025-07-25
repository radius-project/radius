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

package text

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

const defaultText = "test default"
const testPrompt = "test prompt"
const testPlaceholder = "test placeholder"

var (
	// Set this to a big value when debugging.
	waitTimeout = 5 * time.Second
)

func Test_NewTextModel(t *testing.T) {
	options := TextModelOptions{
		Default:     defaultText,
		Placeholder: testPlaceholder,
		Validate: func(input string) error {
			return nil
		},
	}

	model := NewTextModel(testPrompt, options)

	validateNewTextModel(t, &model, &options)
	require.Equal(t, textinput.EchoNormal, model.textInput.EchoMode)
}

func Test_NewTextModel_UpdateEchoMode(t *testing.T) {
	options := TextModelOptions{
		Default:     defaultText,
		Placeholder: testPlaceholder,
		Validate: func(input string) error {
			return nil
		},
		EchoMode: textinput.EchoPassword,
	}

	model := NewTextModel(testPrompt, options)

	validateNewTextModel(t, &model, &options)
	require.Equal(t, textinput.EchoPassword, model.textInput.EchoMode)
}

func validateNewTextModel(t *testing.T, model *Model, options *TextModelOptions) {
	require.NotNil(t, model)
	require.NotNil(t, model.textInput)
	require.Equal(t, testPrompt, model.prompt)
	require.Equal(t, options.Placeholder, model.textInput.Placeholder)
	require.Nil(t, model.textInput.Validate) // See comments in NewTextModel.
}

func Test_E2E(t *testing.T) {
	const expectedPrompt = "\r" + testPrompt + "\n" +
		"\n" +
		"> " + testPlaceholder + "\n" +
		"\n" +
		"(ctrl+c to quit)"

	setup := func(t *testing.T) *teatest.TestModel {
		options := TextModelOptions{
			Default:     defaultText,
			Placeholder: testPlaceholder,
		}
		model := NewTextModel(testPrompt, options)
		return teatest.NewTestModel(t, model, teatest.WithInitialTermSize(18, 50))
	}

	normalizeOutput := func(bts []byte) string {
		return stripansi.Strip(strings.ReplaceAll(string(bts), "\r\n", "\n"))
	}

	waitForContains := func(t *testing.T, reader io.Reader, target string) string {
		normalized := ""
		teatest.WaitFor(t, reader, func(bts []byte) bool {
			normalized = normalizeOutput(bts)
			t.Logf("Testing output:\n\n%s", normalized)
			return strings.Contains(normalized, target)
		}, teatest.WithDuration(waitTimeout))

		return normalized
	}

	waitForInitialRender := func(t *testing.T, reader io.Reader) string {
		return waitForContains(t, reader, ">")
	}

	t.Run("confirm prompt", func(t *testing.T) {
		tm := setup(t)

		output := waitForInitialRender(t, tm.Output())

		require.Equal(t, expectedPrompt, output)
	})

	t.Run("confirm default", func(t *testing.T) {
		tm := setup(t)
		tm.Send(tea.KeyMsg{
			Type: tea.KeyEnter,
		})

		if err := tm.Quit(); err != nil {
			t.Fatal(err)
		}

		// FinalModel only returns once the program has finished running or when it times out.
		// Please see: https://github.com/charmbracelet/x/blob/20117e9c8cd5ad229645f1bca3422b7e4110c96c/exp/teatest/teatest.go#L220.
		// That is why we call tm.Quit() before tm.FinalModel().
		model, ok := tm.FinalModel(t).(Model)
		require.True(t, ok, "Final model should be of type Model")

		require.True(t, model.valueEntered)
		require.False(t, model.Quitting)
		require.Equal(t, defaultText, model.GetValue())
	})

	t.Run("confirm value", func(t *testing.T) {
		const userInputText = "abcd"
		tm := setup(t)
		tm.Type(userInputText)
		tm.Send(tea.KeyMsg{
			Type: tea.KeyEnter,
		})

		if err := tm.Quit(); err != nil {
			t.Fatal(err)
		}

		// FinalModel only returns once the program has finished running or when it times out.
		// Please see: https://github.com/charmbracelet/x/blob/20117e9c8cd5ad229645f1bca3422b7e4110c96c/exp/teatest/teatest.go#L220.
		// That is why we call tm.Quit() before tm.FinalModel().
		model, ok := tm.FinalModel(t).(Model)
		require.True(t, ok, "Final model should be of type Model")

		require.True(t, model.valueEntered)
		require.False(t, model.Quitting)
		require.Equal(t, userInputText, model.GetValue())
	})

	t.Run("cancel", func(t *testing.T) {
		tm := setup(t)
		tm.Send(tea.KeyMsg{
			Type: tea.KeyCtrlC,
		})

		if err := tm.Quit(); err != nil {
			t.Fatal(err)
		}

		// FinalModel only returns once the program has finished running or when it times out.
		// Please see: https://github.com/charmbracelet/x/blob/20117e9c8cd5ad229645f1bca3422b7e4110c96c/exp/teatest/teatest.go#L220.
		// That is why we call tm.Quit() before tm.FinalModel().
		model, ok := tm.FinalModel(t).(Model)
		require.True(t, ok, "Final model should be of type Model")

		require.False(t, model.valueEntered)
		require.True(t, model.Quitting)
		require.Equal(t, defaultText, model.GetValue())
	})
}
