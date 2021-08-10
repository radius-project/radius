// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/process"
	"github.com/stretchr/testify/require"
)

type ProcessExecution struct {
	PID                int
	Executable         string
	Args               []string
	Env                []string
	StartWaitingCalled bool
	StartedAt          time.Time
	EndedAt            time.Time
	ExitHandler        process.ProcessExitHandler
	ExitCode           int
}

type TestProcessExecutor struct {
	nextPID    int32
	Executions []ProcessExecution
}

const (
	NotFound = -1
)

func NewTestProcessExecutor() *TestProcessExecutor {
	return &TestProcessExecutor{
		Executions: make([]ProcessExecution, 0),
	}
}

func (e *TestProcessExecutor) StartProcess(ctx context.Context, exe string, args []string, env []string, handler process.ProcessExitHandler) (pid int, startWaitingForExit func(), err error) {
	err = nil

	newArgs := make([]string, len(args))
	copy(newArgs, args)
	newEnv := make([]string, len(env))
	copy(newEnv, env)

	pid = int(atomic.AddInt32(&e.nextPID, 1))
	pe := ProcessExecution{
		PID:         pid,
		Executable:  exe,
		Args:        newArgs,
		Env:         newEnv,
		StartedAt:   time.Now(),
		ExitHandler: handler,
	}
	e.Executions = append(e.Executions, pe)

	startWaitingForExit = func() {
		i := e.findByPid(pid)
		pe := e.Executions[i]
		pe.StartWaitingCalled = true
		e.Executions[i] = pe
	}

	return
}

func (e *TestProcessExecutor) SimulateProcessExit(t *testing.T, pid int, exitCode int) {
	i := e.findByPid(pid)
	if i == NotFound {
		require.Failf(t, "invalid PID", "no process with given PID %d found (test issue)", pid)
	}
	pe := e.Executions[i]
	pe.ExitCode = exitCode
	pe.EndedAt = time.Now()
	e.Executions[i] = pe
	pe.ExitHandler.OnProcessExited(pid, exitCode)
}

func (e *TestProcessExecutor) FindAll(exeName string) []ProcessExecution {
	var retval []ProcessExecution

	for _, pe := range e.Executions {
		if pe.Executable == exeName {
			retval = append(retval, pe)
		}
	}

	return retval
}

func (e *TestProcessExecutor) findByPid(pid int) int {
	for i, pe := range e.Executions {
		if pe.PID == pid {
			return i
		}
	}

	return NotFound
}
