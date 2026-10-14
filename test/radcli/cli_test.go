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

package radcli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ResourceRegistration_serializesWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the local functional runner requires Linux or macOS")
	}
	directory := t.TempDir()
	t.Setenv("RAD_PATH", directory)
	t.Setenv("REGISTRATION_TEST_STATE", directory)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "rad"), []byte(`#!/bin/sh
set -eu
mkdir "${REGISTRATION_TEST_STATE}/active"
trap 'rmdir "${REGISTRATION_TEST_STATE}/active"' EXIT
sleep 0.05
printf '%s\n' "$*"
`), 0700))

	var workers sync.WaitGroup
	type result struct {
		output string
		err    error
		want   string
	}
	results := make(chan result, 6)
	for index := range 6 {
		workers.Go(func() {
			cli := NewCLI(t, "test-config.yaml")
			if index%2 == 0 {
				output, err := cli.ResourceTypeCreate(t.Context(), "testType", "manifest.yaml")
				results <- result{output: output, err: err, want: "resource-type create --from-file manifest.yaml testType --config test-config.yaml"}
			} else {
				output, err := cli.ResourceProviderCreate(t.Context(), "manifest.yaml")
				results <- result{output: output, err: err, want: "resource-provider create --from-file manifest.yaml --config test-config.yaml"}
			}
		})
	}
	workers.Wait()
	close(results)
	for result := range results {
		require.NoError(t, result.err)
		require.Contains(t, result.output, result.want)
	}
}

func Test_tailLines(t *testing.T) {
	t.Run("returns input unchanged when within limit", func(t *testing.T) {
		out := "line1\nline2\nline3"
		require.Equal(t, out, tailLines(out, 20))
	})

	t.Run("keeps only the trailing lines and marks truncation", func(t *testing.T) {
		var b strings.Builder
		for i := 1; i <= 50; i++ {
			b.WriteString("line")
			b.WriteByte(byte('0' + i%10))
			b.WriteByte('\n')
		}
		// The transport error rad prints appears on the final line.
		b.WriteString(`Error: Get "https://127.0.0.1:37481/...": read: connection reset by peer`)

		got := tailLines(b.String(), 20)

		assert.True(t, strings.HasPrefix(got, "...(output truncated"), "expected a truncation marker prefix")
		assert.Contains(t, got, "connection reset by peer", "must preserve the trailing transport marker")
		// 1 marker line + 20 tail lines.
		assert.Equal(t, 21, len(strings.Split(got, "\n")))
	})
}
