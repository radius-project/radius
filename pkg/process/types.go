// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package process

import (
	"context"
	"os/exec"
	"runtime"
)

const (
	// Invalid PID code is used when process failed to start, or the PID has not been captured yet
	InvalidPID = -1
)

type ProcessData struct {
	PID     int
	Cmdline string
}
type Executor interface {
	StartProcess(ctx context.Context, cmd *exec.Cmd, exitHandler ProcessExitHandler) (pid int, startWaitForProcessExit func(), err error)
	StopProcess(pid int) error
	Processes() ([]ProcessData, error)
}

type ProcessExitHandler interface {
	// Indicates that process with a given PID has finished execution
	// If err is nil, the process exit code was properly captured and the exitCode value is valid
	// if err is not nil, there was a problem tracking the process and the exitCode value is not valid
	OnProcessExited(pid int, exitCode int, err error)
}

func GetExecutableExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	} else {
		return ""
	}
}
