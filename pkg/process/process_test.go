/*
Copyright 2026 The Radius Authors.

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

package process

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const processHelperEnv = "RADIUS_PROCESS_TEST_HELPER"

var commandConstructors = []struct {
	name    string
	command func(string, ...string) *exec.Cmd
}{
	{name: "Command", command: Command},
	{name: "CommandContext", command: func(name string, args ...string) *exec.Cmd {
		return CommandContext(context.Background(), name, args...)
	}},
}

func testCommandInput(t *testing.T) {
	t.Helper()
	for _, constructor := range commandConstructors {
		for _, tt := range []struct {
			name     string
			payload  string
			exitCode int
		}{
			{name: "default EOF"},
			{name: "finite input after construction", payload: "SELECT 1;\n"},
			{name: "nonzero exit", payload: "SELECT 1;\n", exitCode: 7},
		} {
			t.Run(constructor.name+"/"+tt.name, func(t *testing.T) {
				cmd := constructor.command(os.Args[0], "-test.run=^TestProcessHelper$")
				if tt.payload != "" {
					cmd.Stdin = bytes.NewReader([]byte(tt.payload))
				}
				runInputHelper(t, cmd, tt.payload, tt.exitCode)
			})
		}
	}

	t.Run("finite input before configuration", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
		input := bytes.NewReader([]byte("SELECT 2;\n"))
		cmd.Stdin = input
		require.Same(t, cmd, configure(cmd))
		require.Same(t, input, cmd.Stdin)
		runInputHelper(t, cmd, "SELECT 2;\n", 0)
	})
}

func runInputHelper(t *testing.T, cmd *exec.Cmd, want string, exitCode int) {
	t.Helper()
	mode := "echo"
	if exitCode != 0 {
		mode = "echo-error"
	}
	cmd.Env = append(os.Environ(), processHelperEnv+"="+mode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	// Bound plain Command tests too, without changing production lifetimes.
	timer := time.AfterFunc(10*time.Second, func() {
		_ = cmd.Process.Kill()
	})
	defer timer.Stop()
	err := cmd.Wait()
	if exitCode == 0 {
		require.NoError(t, err, stderr.String())
	} else {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, exitCode, exitErr.ExitCode())
	}
	require.Equal(t, want, stdout.String())
	require.Equal(t, "helper stderr", stderr.String())
	require.Equal(t, exitCode, cmd.ProcessState.ExitCode())
}

func testCommandContextCancellation(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), processHelperEnv+"=wait")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make([]byte, len("ready"))
	_, err = io.ReadFull(stdout, ready)
	require.NoError(t, err)
	require.Equal(t, "ready", string(ready))
	cancel()
	require.Error(t, cmd.Wait())
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestProcessHelper(t *testing.T) {
	switch os.Getenv(processHelperEnv) {
	case "echo", "echo-error":
		_, err := io.Copy(os.Stdout, os.Stdin)
		require.NoError(t, err)
		_, err = io.WriteString(os.Stderr, "helper stderr")
		require.NoError(t, err)
		if os.Getenv(processHelperEnv) == "echo-error" {
			os.Exit(7) //nolint:forbidigo // Exercise exec.ExitError without losing the helper's output.
		}
		os.Exit(0) //nolint:forbidigo // Return only the helper output, without the test runner's PASS message.
	case "wait":
		_, err := io.WriteString(os.Stdout, "ready")
		require.NoError(t, err)
		time.Sleep(time.Minute)
		os.Exit(1) //nolint:forbidigo // The parent must cancel this helper before it exits on its own.
	}
}
