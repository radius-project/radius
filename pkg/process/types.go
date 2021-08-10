// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package process

import (
	"context"
)

type IExecutor interface {
	StartProcess(ctx context.Context, exe string, args []string, env []string, exitHandler ProcessExitHandler) (pid int, startWaitForProcessExit func(), err error)
}

type ProcessExitHandler interface {
	OnProcessExited(pid int, exitCode int)
}
