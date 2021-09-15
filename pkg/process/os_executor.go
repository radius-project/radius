// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package process

import (
	"context"
	"os"
	"os/exec"
)

type OSExecutor struct{}

func NewOSExecutor() IExecutor {
	return &OSExecutor{}
}

func (e *OSExecutor) StartProcess(ctx context.Context, exe string, args []string, env []string, handler ProcessExitHandler) (int, func(), error) {
	cmd := exec.Command(exe)
	cmdArgs := make([]string, 1)
	cmdArgs[0] = exe
	cmdArgs = append(cmdArgs, args...)
	cmd.Args = cmdArgs
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	processExited := make(chan struct{}, 1)

	go func() {
		select {
		case <-processExited:
			if ctx.Err() == nil {
				// We did not kill the process and the context has not expired--
				// report process exit code.
				if handler != nil {
					handler.OnProcessExited(cmd.Process.Pid, cmd.ProcessState.ExitCode())
				}
			}
		case <-ctx.Done():
			// Timeout for process run, or we are shutting down.
		}
	}()

	startWaitingForProcessExit := func() {
		go func() {
			cmd.Wait()
			processExited <- struct{}{}
			close(processExited)
		}()
	}

	return cmd.Process.Pid, startWaitingForProcessExit, nil
}

func (e *OSExecutor) StopProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err == nil {
		err = proc.Kill()
	}
	return err
}
