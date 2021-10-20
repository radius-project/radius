// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"

	procutil "github.com/shirou/gopsutil/process"
)

type OSExecutor struct{}

func NewOSExecutor() Executor {
	return &OSExecutor{}
}

func (e *OSExecutor) StartProcess(ctx context.Context, cmd *exec.Cmd, handler ProcessExitHandler) (int, func(), error) {
	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	processExited := make(chan error, 1)

	go func() {
		var err error
		select {
		case err = <-processExited:
			// Do not report anything if the context expired
			if ctx.Err() == nil {
				// We did not kill the process and the context has not expired--
				// report process exit code.
				if handler != nil {
					var ee *exec.ExitError
					if err == nil || errors.As(err, &ee) {
						handler.OnProcessExited(cmd.Process.Pid, cmd.ProcessState.ExitCode(), nil)
					} else {
						handler.OnProcessExited(cmd.Process.Pid, -1, err)
					}
				}
			}
		case <-ctx.Done():
			// Timeout for process run, or we are shutting down.
		}
	}()

	startWaitingForProcessExit := func() {
		go func() {
			err := cmd.Wait()
			processExited <- err
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

func (e *OSExecutor) Processes() ([]ProcessData, error) {
	processes, err := procutil.Processes()
	if err != nil {
		return nil, err
	}

	retval := make([]ProcessData, len(processes))
	for i, p := range processes {
		retval[i].PID = int(p.Pid)

		cmdline, err := p.Cmdline()
		if err != nil {
			return nil, err
		}

		retval[i].Cmdline = cmdline
	}

	return retval, nil
}
