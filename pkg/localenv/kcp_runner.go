// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync/atomic"
	"time"

	"github.com/Azure/radius/pkg/cli/download"
	"github.com/Azure/radius/pkg/process"
	"github.com/go-logr/logr"
	"github.com/gofrs/flock"
	"k8s.io/apimachinery/pkg/util/wait"
)

type runningState uint32
type processCheck int

const (
	running runningState = 1
	ready   runningState = 0

	performProcessCheck processCheck = 1
	skipProcessCheck    processCheck = 0
)

type KcpOptions struct {
	Clean            bool
	Executor         process.Executor
	KubeConfigPath   string
	WorkingDirectory string
	Started          chan<- struct{}
}

type KcpRunner struct {
	log               logr.Logger
	clean             bool
	workingDirectory  string
	kcpExecutablePath string
	kubeConfigPath    string
	processExited     chan finishedProcessInfo
	kcpPid            int
	state             runningState
	processExecutor   process.Executor
	started           chan<- struct{}
}

var _ process.ProcessExitHandler = (*KcpRunner)(nil)

type finishedProcessInfo struct {
	err      error
	exitCode int
}

func NewKcpRunner(log logr.Logger, executablesDir string, options KcpOptions) (*KcpRunner, error) {
	kcpPath := path.Join(executablesDir, kcpFilename())

	pe := options.Executor
	if pe == nil {
		pe = process.NewOSExecutor()
	}

	return &KcpRunner{
		log:               log,
		clean:             options.Clean,
		workingDirectory:  options.WorkingDirectory,
		kcpExecutablePath: kcpPath,
		kubeConfigPath:    options.KubeConfigPath,
		processExecutor:   pe,
		kcpPid:            process.InvalidPID,
		state:             ready,
		started:           options.Started,
	}, nil
}

func (r *KcpRunner) Name() string {
	return "KCP runner"
}

func (r *KcpRunner) Run(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	if !atomic.CompareAndSwapUint32((*uint32)(&r.state), uint32(ready), uint32(running)) {
		return fmt.Errorf("KCP run in progress")
	}
	defer func() { atomic.StoreUint32((*uint32)(&r.state), uint32(ready)) }()

	if _, err := os.Stat(r.kcpExecutablePath); err != nil {
		return fmt.Errorf("unable to locate KCP binary")
	}

	flock := flock.New(path.Join(r.workingDirectory, "radiusd.lock"))
	locked, err := flock.TryLock()
	if err != nil {
		return fmt.Errorf("unable to take radiusd.lock: %w", err)
	} else if !locked {
		return errors.New("kcp is already running")
	}

	// We've taken the lock
	defer flock.Close()
	r.cleanup(skipProcessCheck)

	if err := r.cleanup(performProcessCheck); err != nil {
		return err
	}

	log.Info("Starting API Server...")
	cmd := exec.CommandContext(ctx, r.kcpExecutablePath, "start")
	cmd.Dir = r.workingDirectory
	pid, startWaitForProcessExit, err := r.processExecutor.StartProcess(ctx, cmd, r)
	if err != nil {
		return fmt.Errorf("unable to start KCP process: %w", err)
	}

	r.kcpPid = pid
	defer func() { r.kcpPid = process.InvalidPID }()
	r.processExited = make(chan finishedProcessInfo, 1)
	startWaitForProcessExit()

	err = r.waitKubeConfigReady(ctx)
	if err != nil {
		return err
	}

	close(r.started)
	log.Info("Started API Server")

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
	if !r.clean {
		return nil
	}

	kcpConfigPath := path.Join(r.workingDirectory, ".kcp")

	// Make sure the data from previous run was deleted
	if _, err := os.Stat(kcpConfigPath); err == nil {
		if err := os.RemoveAll(kcpConfigPath); err != nil {
			return fmt.Errorf("unable to clean up old KCP run data: %w", err)
		}
	}

	return nil
}

func kcpFilename() string {
	return "kcp" + process.GetExecutableExt()
}

func (r *KcpRunner) waitKubeConfigReady(ctx context.Context) error {
	waitProcessesStarted := func() (bool, error) {
		if _, err := os.Stat(r.kubeConfigPath); err == nil {
			return true, nil
		} else {
			if os.IsNotExist(err) {
				return false, nil
			} else {
				return false, err
			}
		}
	}

	return wait.PollUntil(500*time.Millisecond, waitProcessesStarted, ctx.Done())
}
