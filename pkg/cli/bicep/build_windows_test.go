//go:build windows

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

package bicep

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const helperProcessEnvVar = "GO_WANT_BICEP_STDERR_HELPER_PROCESS"

func TestVersion_ClosesCallerStderrPipePromptly(t *testing.T) {
	if os.Getenv(helperProcessEnvVar) == "1" {
		version := Version()
		if version != "0.42.1" {
			fmt.Fprintf(os.Stderr, "unexpected version: %s", version)
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, "helper completed")
		return
	}

	dir := t.TempDir()
	bicepPath := filepath.Join(dir, "bicep.cmd")
	err := os.WriteFile(bicepPath, []byte(`@echo off
if "%1"=="--version" (
  echo bicep 0.42.1
  echo fake bicep stderr 1>&2
  exit /b 0
)

echo unexpected args: %* 1>&2
exit /b 1
`), 0o600)
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run", "^TestVersion_ClosesCallerStderrPipePromptly$")
	cmd.Env = append(os.Environ(), helperProcessEnvVar+"=1", BicepEnvVar+"="+bicepPath)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	stderrDone := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stderr)
		stderrDone <- data
	}()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case err = <-waitDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("helper process did not exit")
	}

	select {
	case stderrOutput := <-stderrDone:
		require.Contains(t, string(bytes.TrimSpace(stderrOutput)), "helper completed")
	case <-time.After(10 * time.Second):
		t.Fatal("stderr pipe did not reach EOF")
	}
}
