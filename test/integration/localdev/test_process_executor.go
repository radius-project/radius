// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"
	"sync"
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
	m          *sync.RWMutex
}

const (
	NotFound              = -1
	KilledProcessExitCode = 137 // 128 + SIGKILL (9)
)

func NewTestProcessExecutor() *TestProcessExecutor {
	return &TestProcessExecutor{
		Executions: make([]ProcessExecution, 0),
		m:          &sync.RWMutex{},
	}
}

func (e *TestProcessExecutor) StartProcess(ctx context.Context, exe string, args []string, env []string, handler process.ProcessExitHandler) (pid int, startWaitingForExit func(), err error) {
	err = nil

	newArgs := make([]string, len(args))
	copy(newArgs, args)
	newEnv := make([]string, len(env))
	copy(newEnv, env)

	pid = int(atomic.AddInt32(&e.nextPID, 1))
	e.m.Lock()
	defer e.m.Unlock()

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

// Called by the controller (via Executor interface)
func (e *TestProcessExecutor) StopProcess(pid int) error {
	return e.stopProcessImpl(pid, KilledProcessExitCode)
}

// Called by tests
func (e *TestProcessExecutor) SimulateProcessExit(t *testing.T, pid int, exitCode int) {
	err := e.stopProcessImpl(pid, exitCode)
	if err != nil {
		require.Failf(t, "invalid PID (test issue)", err.Error())
	}
}

func (e *TestProcessExecutor) FindAll(exeName string, cond func(pe ProcessExecution) bool) []ProcessExecution {
	retval := make([]ProcessExecution, 0)
	e.m.RLock()
	defer e.m.RUnlock()

	for _, pe := range e.Executions {
		if pe.Executable == exeName {
			include := true
			if cond != nil {
				include = cond(pe)
			}

			if include {
				retval = append(retval, pe)
			}
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

func (e *TestProcessExecutor) stopProcessImpl(pid, exitCode int) error {
	e.m.Lock()
	defer e.m.Unlock()

	i := e.findByPid(pid)
	if i == NotFound {
		return fmt.Errorf("No process with PID %d found", pid)
	}
	pe := e.Executions[i]
	pe.ExitCode = exitCode
	pe.EndedAt = time.Now()
	e.Executions[i] = pe
	if pe.ExitHandler != nil {
		pe.ExitHandler.OnProcessExited(pid, exitCode, nil)
	}
	return nil
}
