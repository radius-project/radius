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

package output

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type writerInput struct {
	Size   string
	IsCool bool
}

// Just a single E2E test for the Write method, writers are tested in detail elsewhere.
func Test_Write(t *testing.T) {
	obj := writerInput{
		Size:   "mega",
		IsCool: true,
	}

	buffer := &bytes.Buffer{}
	err := Write(FormatJson, obj, buffer, FormatterOptions{})
	require.NoError(t, err)

	expected := `{
  "Size": "mega",
  "IsCool": true
}
`
	require.Equal(t, expected, buffer.String())
}

func Test_OutputWriter_SeparateLogs(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	writer := &OutputWriter{Writer: &stdout, LogWriter: &stderr}
	writer.LogInfo("Listing %s...", "resources")
	step := writer.BeginStep("Loading %d resources", 1)

	expected := writerInput{Size: "mega", IsCool: true}
	require.NoError(t, writer.WriteFormatted(FormatJson, expected, FormatterOptions{}))
	writer.CompleteStep(step)
	writer.LogInfo("Done")

	var actual writerInput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	require.Equal(t, expected, actual)
	require.Equal(t, "Listing resources...\nLoading 1 resources\nDone\n", stderr.String())
}

func Test_OutputWriter_DefaultLogs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	writer := &OutputWriter{Writer: &stdout}
	writer.LogInfo("Listing %s...", "resources")
	step := writer.BeginStep("Loading %d resources", 1)
	writer.CompleteStep(step)

	require.Equal(t, "Listing resources...\nLoading 1 resources\n", stdout.String())
}

func Test_LogInfo_Destinations(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	originalStdout, originalStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		SetLogWriter(nil)
		os.Stdout, os.Stderr = originalStdout, originalStderr
		require.NoError(t, stdout.Close())
		require.NoError(t, stderr.Close())
	})
	os.Stdout, os.Stderr = stdout, stderr

	SetLogWriter(stderr)
	LogInfo("Listing %s...", "resources")
	step := BeginStep("Loading %d resources", 1)
	CompleteStep(step)
	SetLogWriter(nil)
	LogInfo("Default stdout")

	data, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	require.Equal(t, "Default stdout\n", string(data))
	data, err = os.ReadFile(stderr.Name())
	require.NoError(t, err)
	require.Equal(t, "Listing resources...\nLoading 1 resources\n", string(data))
}
