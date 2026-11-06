//go:build windows

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

package kubeenv

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestEnvironmentChildProcess(t *testing.T) {
	if os.Getenv("RADIUS_ENVTEST_CHILD") == "1" {
		time.Sleep(time.Minute)
	}
}

func TestCaptureNewProcessesIsolatesEnvironment(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	existing := startTestChild(t, executable)
	before, err := childProcessIDs()
	require.NoError(t, err)

	owned := startTestChild(t, executable)
	contents, err := os.ReadFile(executable)
	require.NoError(t, err)
	otherExecutable := filepath.Join(t.TempDir(), "unrelated.exe")
	require.NoError(t, os.WriteFile(otherExecutable, contents, 0700))
	unrelated := startTestChild(t, otherExecutable)

	handles, err := captureNewProcesses(before, []string{executable})
	stop := sync.OnceValue(func() error { return stopProcesses(handles) })
	t.Cleanup(func() { require.NoError(t, stop()) })
	require.NoError(t, err)
	require.Len(t, handles, 1, "only the newly started child with the exact executable path belongs to this environment")
	require.NoError(t, stop())

	result, err := windows.WaitForSingleObject(owned, 0)
	require.NoError(t, err)
	require.Equal(t, uint32(windows.WAIT_OBJECT_0), result)
	for _, handle := range []windows.Handle{existing, unrelated} {
		result, err := windows.WaitForSingleObject(handle, 0)
		require.NoError(t, err)
		require.Equal(t, uint32(windows.WAIT_TIMEOUT), result, "unrelated children must remain running")
	}
}

func startTestChild(t *testing.T, executable string) windows.Handle {
	t.Helper()
	cmd := exec.Command(executable, "-test.run=^TestEnvironmentChildProcess$")
	cmd.Env = append(os.Environ(), "RADIUS_ENVTEST_CHILD=1")
	require.NoError(t, cmd.Start())
	var handle windows.Handle
	t.Cleanup(func() {
		if handle == 0 {
			require.NoError(t, cmd.Process.Kill())
		} else {
			require.NoError(t, stopProcesses([]windows.Handle{handle}))
		}
		var exitError *exec.ExitError
		err := cmd.Wait()
		require.True(t, err == nil || errors.As(err, &exitError), "waiting for test child: %v", err)
	})
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	require.NoError(t, err)
	return handle
}
