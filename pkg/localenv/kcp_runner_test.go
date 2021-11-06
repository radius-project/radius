// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/Azure/radius/test/localenvtest"
	"github.com/Azure/radius/test/testcontext"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestRunnerStartsStopsKcp(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)
	executor := localenvtest.NewTestProcessExecutor()
	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	var runnerErr error
	runnerDone := make(chan struct{})
	ctx, cancel := testcontext.GetContext(t)

	go func() {
		runnerErr = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	cancel()
	<-runnerDone
	require.Nil(t, runnerErr)

	kcpExecutions := executor.FindAll(kcpBinaryPath, nil)
	require.Len(t, kcpExecutions, 1)
	require.Truef(t, kcpExecutions[0].Finished(), "runner should have ended KCP process")
}

func TestErrorsIfKcpBinaryNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// No "KCP binary" added

	runner, err := NewKcpRunner(logr.Discard(), tmpDir, KcpOptions{})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, _ := testcontext.GetContext(t)
	err = runner.Run(ctx)
	require.Errorf(t, err, "no KCP binary should result in runner creation error")
}

func TestNoConcurrentKcpRuns(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)
	executor := localenvtest.NewTestProcessExecutor()
	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, cancel := testcontext.GetContext(t)
	runnerDone := make(chan struct{})

	go func() {
		_ = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	err = runner.Run(ctx)
	require.Errorf(t, err, "second run should fail when first run is in progress")
	cancel()
	<-runnerDone

	// Once a run is finished, another run should be possible
	var runnerErr error
	runnerDone = make(chan struct{})
	ctx, cancel = testcontext.GetContext(t)

	go func() {
		runnerErr = runner.Run(ctx)
		close(runnerDone)
	}()

	cancel()
	<-runnerDone
	require.Nil(t, runnerErr)
}

func TestCleansWorkingDirAfterRun(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)
	executor := localenvtest.NewTestProcessExecutor()
	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, cancel := testcontext.GetContext(t)
	runnerDone := make(chan struct{})

	go func() {
		_ = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	// Simulate KCP creating working dir
	kcpWorkingDirPath := path.Join(kcpBinaryDir, ".kcp")
	err = os.Mkdir(kcpWorkingDirPath, os.ModeDir)
	require.NoErrorf(t, err, "unable to simulate creation of KCP working directory")

	cancel()
	<-runnerDone

	require.NoDirExistsf(t, kcpWorkingDirPath, "KcpRunner should have removed KCP working dir")
}

func TestCleansWorkingDirBeforeRun(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)

	// Simulate existence of KCP working dir
	kcpWorkingDirPath := path.Join(kcpBinaryDir, ".kcp")
	err := os.Mkdir(kcpWorkingDirPath, os.ModeDir)
	require.NoErrorf(t, err, "unable to simulate creation of KCP working directory")

	executor := localenvtest.NewTestProcessExecutor()
	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, cancel := testcontext.GetContext(t)
	runnerDone := make(chan struct{})

	go func() {
		_ = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	// The runner should remove the old KCP working dir, but creation of the new working dir
	// belongs to the KCP process, which we do not really start here.
	// Net effect is, we expect the working dir to not exist after the run has started.
	require.NoDirExistsf(t, kcpWorkingDirPath, "KcpRunner should have removed KCP working dir")

	cancel()
	<-runnerDone
}

func TestRunFailsIfKcpAlreadyRunning(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)
	executor := localenvtest.NewTestProcessExecutor()
	cmd := exec.Command(kcpBinaryPath)
	cmd.Args = []string{kcpBinaryPath, "start"}
	cmd.Dir = kcpBinaryDir
	pid, _, err := executor.StartProcess(context.Background(), cmd, nil)
	require.NoError(t, err)

	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, cancel := testcontext.GetContext(t)
	err = runner.Run(ctx)
	require.Errorf(t, err, "runner should fail because KCP is already running")

	executor.SimulateProcessExit(t, pid, 0)

	// Once the other KCP process exits, the runner should be able to start KCP successfully
	var runnerErr error
	runnerDone := make(chan struct{})
	go func() {
		runnerErr = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	cancel()
	<-runnerDone
	require.NoErrorf(t, runnerErr, "run should be successful")
}

func TestRunFailsIfKcpCrashes(t *testing.T) {
	t.Parallel()

	kcpBinaryDir, kcpBinaryPath := createKcpTestDir(t)
	executor := localenvtest.NewTestProcessExecutor()

	runner, err := NewKcpRunner(logr.Discard(), kcpBinaryDir, KcpOptions{Executor: executor})
	require.NoErrorf(t, err, "unable to create KcpRunner")

	ctx, _ := testcontext.GetContext(t)
	runnerDone := make(chan struct{})
	var runnerErr error

	go func() {
		runnerErr = runner.Run(ctx)
		close(runnerDone)
	}()
	err = executor.WaitProcessesStarted(ctx, kcpBinaryPath, 1)
	require.NoErrorf(t, err, "KCP process was not started")

	kcpExecutions := executor.FindAll(kcpBinaryPath, nil)
	require.Len(t, kcpExecutions, 1)
	require.True(t, !kcpExecutions[0].Finished())

	executor.SimulateProcessExit(t, kcpExecutions[0].PID, 1)
	<-runnerDone

	require.Errorf(t, runnerErr, "run should have ended with error becasue KCP 'crashed'")
}

func createKcpTestDir(t *testing.T) (string, string) {
	kcpBinaryDir := t.TempDir()
	kcpBinaryPath := path.Join(kcpBinaryDir, "kcp")
	err := ioutil.WriteFile(kcpBinaryPath, []byte("kcp binary"), 0)
	if err != nil {
		t.Fatalf("unable to simulate KCP binary directory: %v", err)
	}

	return kcpBinaryDir, kcpBinaryPath
}
