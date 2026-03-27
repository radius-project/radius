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

package status

import (
	"bytes"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/terraform/installer"
	"github.com/stretchr/testify/require"
)

func Test_statusFormat(t *testing.T) {
	format := statusFormat()
	require.NotNil(t, format.Columns)
	require.Len(t, format.Columns, 4)

	// Verify all expected columns are present (concise view, use --output json for full details)
	columnHeadings := make([]string, len(format.Columns))
	for i, col := range format.Columns {
		columnHeadings[i] = col.Heading
	}

	expectedHeadings := []string{
		"STATE",
		"VERSION",
		"LAST ERROR",
		"LAST UPDATED",
	}

	require.Equal(t, expectedHeadings, columnHeadings)
}

func Test_statusFormat_TableOutput(t *testing.T) {
	// Test that the JSONPath expressions work with the actual struct
	installedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	lastUpdated := time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC)

	status := &installer.StatusResponse{
		State:          installer.ResponseStateReady,
		CurrentVersion: "1.6.4",
		BinaryPath:     "/terraform/versions/1.6.4/terraform",
		InstalledAt:    &installedAt,
		Source: &installer.SourceInfo{
			URL:      "https://releases.hashicorp.com/terraform/1.6.4/terraform_1.6.4_linux_amd64.zip",
			Checksum: "sha256:abc123",
		},
		Queue: &installer.QueueInfo{
			Pending: 0,
		},
		LastUpdated: lastUpdated,
	}

	format := statusFormat()
	formatter := &output.TableFormatter{}
	buf := &bytes.Buffer{}

	err := formatter.Format(status, buf, format)
	require.NoError(t, err)

	tableOutput := buf.String()

	// Verify key values appear in table output (concise view shows only essential columns)
	require.Contains(t, tableOutput, "STATE")
	require.Contains(t, tableOutput, "VERSION")
	require.Contains(t, tableOutput, "ready")
	require.Contains(t, tableOutput, "1.6.4")
}

func Test_statusFormat_TableOutput_NotInstalled(t *testing.T) {
	status := &installer.StatusResponse{
		State:          installer.ResponseStateNotInstalled,
		CurrentVersion: "",
		BinaryPath:     "",
		InstalledAt:    nil,
		Source:         nil,
		Queue:          nil,
		LastUpdated:    time.Time{},
	}

	format := statusFormat()
	formatter := &output.TableFormatter{}
	buf := &bytes.Buffer{}

	err := formatter.Format(status, buf, format)
	require.NoError(t, err)

	tableOutput := buf.String()
	require.Contains(t, tableOutput, "STATE")
	require.NotContains(t, tableOutput, "<no value>")
}

func Test_emptyIfNoValueTransformer(t *testing.T) {
	transformer := &emptyIfNoValueTransformer{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no value marker returns dash",
			input:    "<no value>",
			expected: "-",
		},
		{
			name:     "nil marker returns dash",
			input:    "<nil>",
			expected: "-",
		},
		{
			name:     "empty string returns dash",
			input:    "",
			expected: "-",
		},
		{
			name:     "whitespace only returns dash",
			input:    "   ",
			expected: "-",
		},
		{
			name:     "zero time returns dash",
			input:    "0001-01-01T00:00:00Z",
			expected: "-",
		},
		{
			name:     "quoted zero time returns dash",
			input:    "\"0001-01-01T00:00:00Z\"",
			expected: "-",
		},
		{
			name:     "normal value is preserved",
			input:    "1.6.4",
			expected: "1.6.4",
		},
		{
			name:     "path value is preserved",
			input:    "/terraform/versions/1.6.4/terraform",
			expected: "/terraform/versions/1.6.4/terraform",
		},
		{
			name:     "state value is preserved",
			input:    "ready",
			expected: "ready",
		},
		{
			name:     "timestamp is preserved",
			input:    "2024-01-15T10:30:00Z",
			expected: "2024-01-15T10:30:00Z",
		},
		{
			name:     "quoted timestamp has quotes stripped",
			input:    "\"2024-01-15T10:30:00Z\"",
			expected: "2024-01-15T10:30:00Z",
		},
		{
			name:     "zero number is preserved",
			input:    "0",
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformer.Transform(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
