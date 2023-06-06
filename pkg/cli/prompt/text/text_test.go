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

var (
	// Set this to a big value when debugging.
	waitTimeout = 1 * time.Second
)

func Test_NewTextModel(t *testing.T) {
	options := TextModelOptions{
		Default:     "test default",
		Placeholder: "test placeholder",
		Validate: func(input string) error {
			return nil
		},
	}
	model := NewTextModel("test prompt", options)
	require.NotNil(t, model)
	require.NotNil(t, model.textInput)

	require.Equal(t, "test prompt", model.prompt)
	require.Equal(t, options.Placeholder, model.textInput.Placeholder)
	require.Equal(t, textinput.EchoNormal, model.textInput.EchoMode)
	require.Nil(t, model.textInput.Validate) // See comments in NewTextModel.
}

func Test_NewTextModel_UpdateEchoMode(t *testing.T) {
	options := TextModelOptions{
		Default:     "test default",
		Placeholder: "test placeholder",
		Validate: func(input string) error {
			return nil
		},
		EchoMode: textinput.EchoPassword,
	}
	model := NewTextModel("test prompt", options)
	require.NotNil(t, model)
	require.NotNil(t, model.textInput)

	require.Equal(t, "test prompt", model.prompt)
	require.Equal(t, options.Placeholder, model.textInput.Placeholder)
	require.Equal(t, textinput.EchoPassword, model.textInput.EchoMode)
	require.Nil(t, model.textInput.Validate) // See comments in NewTextModel.
}

func Test_E2E(t *testing.T) {
	// Note: unfortunately I ran into bugs with the testing framework while trying to test more advance
	// scenarios like validation. The output coming from the framework was truncated, so I just couldn't do it :(.
	//
	// At the time of writing the test framework is new and unsupported. We should try again when its more mature.

	setup := func(t *testing.T) *teatest.TestModel {
		options := TextModelOptions{
			Default:     "test default",
			Placeholder: "test placeholder",
		}
		model := NewTextModel("test prompt", options)
		return teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 50))
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

	t.Run("confirm default", func(t *testing.T) {
		tm := setup(t)
		output := waitForInitialRender(t, tm.Output())

		expected := "test prompt\n" +
			"\n" +
			"> test placeholder\n" +
			"\n" +
			"(ctrl+c to quit)"
		require.Equal(t, expected, output)

		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
		tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
		bts, err := io.ReadAll(tm.FinalOutput(t))
		require.NoError(t, err)

		output = normalizeOutput(bts)
		require.Empty(t, strings.TrimSpace(output)) // Output sometimes contains a single space.
		require.True(t, tm.FinalModel(t).(Model).valueEntered)
		require.False(t, tm.FinalModel(t).(Model).Quitting)
		require.Equal(t, "test default", tm.FinalModel(t).(Model).GetValue())
	})

	t.Run("confirm value", func(t *testing.T) {
		tm := setup(t)
		output := waitForInitialRender(t, tm.Output())

		expected := "test prompt\n" +
			"\n" +
			"> test placeholder\n" +
			"\n" +
			"(ctrl+c to quit)"
		require.Equal(t, expected, output)

		tm.Type("abcd")
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
		tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
		bts, err := io.ReadAll(tm.FinalOutput(t))
		require.NoError(t, err)

		output = normalizeOutput(bts)
		require.Empty(t, strings.TrimSpace(output)) // Output sometimes contains a single space.
		require.True(t, tm.FinalModel(t).(Model).valueEntered)
		require.False(t, tm.FinalModel(t).(Model).Quitting)
		require.Equal(t, "abcd", tm.FinalModel(t).(Model).GetValue())
	})

	t.Run("cancel", func(t *testing.T) {
		tm := setup(t)
		output := waitForInitialRender(t, tm.Output())

		expected := "test prompt\n" +
			"\n" +
			"> test placeholder\n" +
			"\n" +
			"(ctrl+c to quit)"
		require.Equal(t, expected, output)

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
		bts, err := io.ReadAll(tm.FinalOutput(t))
		require.NoError(t, err)

		output = normalizeOutput(bts)
		require.Empty(t, strings.TrimSpace(output)) // Output sometimes contains a single space.
		require.False(t, tm.FinalModel(t).(Model).valueEntered)
		require.True(t, tm.FinalModel(t).(Model).Quitting)
		require.Equal(t, "test default", tm.FinalModel(t).(Model).GetValue())
	})
}
