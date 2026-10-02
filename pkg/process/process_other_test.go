//go:build !windows

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

package process

import (
	"bytes"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommands_UnchangedOnNonWindows(t *testing.T) {
	t.Parallel()
	require.False(t, IsWindowless())

	for _, constructor := range commandConstructors {
		t.Run(constructor.name, func(t *testing.T) {
			cmd := constructor.command("test-command", "argument")
			require.Equal(t, []string{"test-command", "argument"}, cmd.Args)
			require.Nil(t, cmd.SysProcAttr)
			require.Nil(t, cmd.Stdin)
		})
	}
	testCommandInput(t)
	t.Run("context cancellation", testCommandContextCancellation)
}

func TestConfigure_UnchangedOnNonWindows(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("test-command")
	attrs := &syscall.SysProcAttr{}
	input := bytes.NewReader([]byte("SELECT 1;\n"))
	cmd.SysProcAttr = attrs
	cmd.Stdin = input
	expected := *cmd

	require.Same(t, cmd, configure(cmd))
	require.Equal(t, expected, *cmd)
	require.Same(t, attrs, cmd.SysProcAttr)
	require.Same(t, input, cmd.Stdin)
}
