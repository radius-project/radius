// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package process

import (
	"context"
	"os/exec"
)

type Executor interface {
	// CONSIDER making this call accept a exec.Cmd instance instead
	StartProcess(ctx context.Context, cmd *exec.Cmd, exitHandler ProcessExitHandler) (pid int, startWaitForProcessExit func(), err error)

	StopProcess(pid int) error
}

type ProcessExitHandler interface {
	// Indicates that process with a given PID has finished execution
	// If err is nil, the process exit code was properly captured and the exitCode value is valid
	// if err is not nil, there was a problem tracking the process and the exitCode value is not valid
	OnProcessExited(pid int, exitCode int, err error)
}
