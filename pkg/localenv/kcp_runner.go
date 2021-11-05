// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync/atomic"

	"github.com/Azure/radius/pkg/cli/download"
	"github.com/Azure/radius/pkg/process"
)

type runningState uint32
type processCheck int

const (
	running runningState = 1
	ready   runningState = 0

	performProcessCheck processCheck = 1
	skipProcessCheck    processCheck = 0
)

type KcpRunner struct {
	kcpExecutablePath string
	processExited     chan finishedProcessInfo
	kcpPid            int
	state             runningState
	processExecutor   process.Executor
}

var _ process.ProcessExitHandler = (*KcpRunner)(nil)

type finishedProcessInfo struct {
	err      error
	exitCode int
}

func NewKcpRunner(executablesDir string, pe process.Executor) (*KcpRunner, error) {
	kcpPath := path.Join(executablesDir, kcpFilename())

	if pe == nil {
		pe = process.NewOSExecutor()
	}

	return &KcpRunner{
		kcpExecutablePath: kcpPath,
		processExecutor:   pe,
		kcpPid:            process.InvalidPID,
		state:             ready,
	}, nil
}

func (r *KcpRunner) Name() string {
	return "KCP runner"
}

func (r *KcpRunner) Run(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32((*uint32)(&r.state), uint32(ready), uint32(running)) {
		return fmt.Errorf("KCP run in progress")
	}
	defer func() { atomic.StoreUint32((*uint32)(&r.state), uint32(ready)) }()

	if _, err := os.Stat(r.kcpExecutablePath); err != nil {
		return fmt.Errorf("unable to locate KCP binary")
	}

	if err := r.cleanup(performProcessCheck); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, r.kcpExecutablePath)
	cmd.Args = []string{r.kcpExecutablePath, "start"}
	cmd.Dir = path.Dir(r.kcpExecutablePath)
	pid, startWaitForProcessExit, err := r.processExecutor.StartProcess(ctx, cmd, r)
	if err != nil {
		return fmt.Errorf("unable to start KCP process: %w", err)
	}

	r.kcpPid = pid
	defer func() { r.kcpPid = process.InvalidPID }()
	r.processExited = make(chan finishedProcessInfo, 1)
	startWaitForProcessExit()

	select {
	case processInfo := <-r.processExited:
		if ctx.Err() == nil {
			if processInfo.err == nil {
				return fmt.Errorf("KCP process exited unexpectedly. Exit code was: %d", processInfo.exitCode)
			} else {
				return fmt.Errorf("KCP process tracking failed: %w", err)
			}
		} else {
			// KCP was ended because context was cancelled (we are shutting down)
			return r.cleanup(skipProcessCheck)
		}
	case <-ctx.Done():
		_ = r.processExecutor.StopProcess(r.kcpPid)
		return r.cleanup(skipProcessCheck)
	}
}

func (r *KcpRunner) OnProcessExited(pid int, exitCode int, err error) {
	if pid == r.kcpPid {
		r.processExited <- finishedProcessInfo{
			err:      err,
			exitCode: exitCode,
		}
		close(r.processExited)
	}
}

func (r *KcpRunner) EnsureKcpExecutable(ctx context.Context) error {
	_, err := os.Stat(r.kcpExecutablePath)
	if err == nil {
		return nil // KCP executable exists
	}

	err = download.Binary(ctx, "kcp", r.kcpExecutablePath)
	if err != nil {
		return err
	}

	return nil
}

func (r *KcpRunner) cleanup(pc processCheck) error {
	kcpConfigPath := path.Join(path.Dir(r.kcpExecutablePath), ".kcp")

	if pc == performProcessCheck {
		kcpRunning, err := r.isKcpRunning()
		if err != nil {
			return fmt.Errorf("unable to determine whether KCP process is running: %w", err)
		}

		if kcpRunning {
			return fmt.Errorf("KCP process is running")
		}
	}

	// Make sure the data from previous run was deleted
	if _, err := os.Stat(kcpConfigPath); err == nil {
		if err := os.RemoveAll(kcpConfigPath); err != nil {
			return fmt.Errorf("unable to clean up old KCP run data: %w", err)
		}
	}

	return nil
}

func (r *KcpRunner) isKcpRunning() (bool, error) {
	if r.kcpPid != process.InvalidPID {
		return true, nil
	}

	processes, err := r.processExecutor.Processes()
	if err != nil {
		return false, err
	}

	for _, p := range processes {
		if strings.Contains(p.Cmdline, r.kcpExecutablePath) {
			return true, nil
		}
	}

	return false, nil
}

func kcpFilename() string {
	return "kcp" + process.GetExecutableExt()
}
