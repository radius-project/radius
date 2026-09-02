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

package process

import (
	"context"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestCommand_NoWindowDisabled(t *testing.T) {
	t.Setenv(NoWindowEnvVar, "")

	cmd := Command("test-command")

	require.Nil(t, cmd.SysProcAttr)
}

func TestCommand_NoWindowEnabled(t *testing.T) {
	t.Setenv(NoWindowEnvVar, "TRUE")

	cmd := Command("test-command")

	require.True(t, cmd.SysProcAttr.HideWindow)
	require.Equal(t, uint32(windows.CREATE_NO_WINDOW), cmd.SysProcAttr.CreationFlags)
}

func TestCommandContext_NoWindowEnabled(t *testing.T) {
	t.Setenv(NoWindowEnvVar, "true")

	cmd := CommandContext(context.Background(), "test-command")

	require.True(t, cmd.SysProcAttr.HideWindow)
	require.Equal(t, uint32(windows.CREATE_NO_WINDOW), cmd.SysProcAttr.CreationFlags)
}

func TestConfigure_PreservesCreationFlags(t *testing.T) {
	t.Setenv(NoWindowEnvVar, "true")
	cmd := exec.Command("test-command")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}

	configure(cmd)

	require.True(t, cmd.SysProcAttr.HideWindow)
	require.Equal(t, uint32(windows.CREATE_NEW_PROCESS_GROUP|windows.CREATE_NO_WINDOW), cmd.SysProcAttr.CreationFlags)
}
