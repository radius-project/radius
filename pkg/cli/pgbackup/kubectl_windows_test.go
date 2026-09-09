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

package pgbackup

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestKubectl_ConsolePolicy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags uint32
		tests string
	}{
		{name: "windowless", flags: windows.CREATE_NO_WINDOW, tests: "^TestKubectlHelper$"},
		{name: "attached", flags: windows.CREATE_NEW_CONSOLE, tests: "^TestKubectlHelper$/^(Always|broken_config)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+tt.tests, "-test.v")
			cmd.Env = append(os.Environ(), helperPolicyEnv+"="+tt.name)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: tt.flags}
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
		})
	}
}

func TestKubectlHelper(t *testing.T) {
	if os.Getenv(helperPolicyEnv) == "" {
		t.Skip("invoked by the console-policy test in an isolated process")
	}
	if os.Getenv(helperPolicyEnv) == "windowless" {
		// CREATE_NO_WINDOW can still supply a hidden console. Match an automation
		// caller with no console attachment, as the existing windowless tests do.
		freeConsole := windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")
		result, _, err := freeConsole.Call()
		require.NotZero(t, result, "FreeConsole: %v", err)
	}
	job, err := windows.CreateJobObject(nil, nil)
	require.NoError(t, err)
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job)
		t.Fatal(err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		t.Fatal(err)
	}
	// Keep the only job handle until process exit so a test-safety timeout also
	// terminates any owned kubectl child, without breakaway or detached execution.
	testKubectl(t)
}

func helperConsoleWindow() bool {
	getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	window, _, _ := getConsoleWindow.Call()
	return window != 0
}
