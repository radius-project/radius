//go:build windows

/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bicep

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// bicepHelperModeEnv selects which role the helper subprocess plays.
	// Set to "rad" to act as the rad-like process that calls Version().
	bicepHelperModeEnv = "GO_BICEP_HELPER_MODE"

	// bicepReadyFileEnv is the path of a file the fake bicep creates once it has
	// started and written its output.  The helper polls for this file so it knows
	// bicep is alive before it exits.
	bicepReadyFileEnv = "GO_BICEP_READY_FILE"

	// bicepReleaseFileEnv is the path of a file the test creates to signal the
	// fake bicep that it may exit.  This prevents orphan batch processes.
	bicepReleaseFileEnv = "GO_BICEP_RELEASE_FILE"
)

// TestHelperProcess is a subprocess entry point for helper-process tests in
// this package.  It is invoked by the test binary itself (via exec.Command) when
// GO_BICEP_HELPER_MODE is set; in all other contexts it returns immediately.
//
// When GO_BICEP_HELPER_MODE=rad it simulates a rad-like process:
//   - it starts Version() in a goroutine (which will block inside runBicepRaw
//     waiting for the fake bicep to exit), and
//   - once the fake bicep signals it has started (by creating GO_BICEP_READY_FILE)
//     the helper writes a message to stderr and calls os.Exit(0), leaving fake
//     bicep alive as an orphan process.
//
// With the OLD implementation (c.Stderr = os.Stderr) fake bicep would inherit
// the caller's stderr handle; the caller's pipe would therefore stay open even
// after the helper exited.  With the fixed implementation (StderrPipe) fake
// bicep never receives the caller's handle, so the caller sees EOF promptly.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(bicepHelperModeEnv) != "rad" {
		return
	}

	// Start Version() asynchronously.  runBicepRaw blocks inside c.Wait() until
	// fake bicep exits (or this process is killed).
	go func() {
		_ = Version()
	}()

	// Poll until fake bicep has started and written its output.
	readyFile := os.Getenv(bicepReadyFileEnv)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := os.Stat(readyFile); err != nil {
		fmt.Fprintf(os.Stderr, "fake bicep did not signal readiness within deadline: %v\n", err)
		os.Exit(2)
	}
	// Exit while fake bicep may still be alive.  This is the crux of the test:
	// any stderr handle that bicep inherited will keep the caller's pipe open
	// after we exit; with StderrPipe() bicep never has the caller's handle, so
	// the pipe closes as soon as we do.
	fmt.Fprintln(os.Stderr, "rad helper exiting")
	os.Exit(0)
}

// TestVersion_ClosesCallerStderrPipePromptly is a Windows-only regression test
// for https://github.com/radius-project/radius/issues/12516.
//
// It verifies that a process that pipes rad's stderr observes EOF on that pipe
// promptly after rad exits — even when the fake bicep child process is still
// alive.
//
// With the OLD implementation (c.Stderr = os.Stderr), bicep inherits the
// caller's stderr handle, so io.ReadAll on the caller's pipe blocks until
// bicep exits.  With the fixed implementation (StderrPipe), bicep never
// receives the caller's handle, so the pipe reaches EOF as soon as rad exits.
func TestVersion_ClosesCallerStderrPipePromptly(t *testing.T) {
	dir := t.TempDir()
	readyFile := filepath.Join(dir, "bicep.ready")
	releaseFile := filepath.Join(dir, "bicep.release")

	// Release fake bicep on test exit so we don't leave orphan processes.
	t.Cleanup(func() {
		_ = os.WriteFile(releaseFile, []byte("release"), 0o600)
	})

	// Write a batch-file fake bicep that:
	//   1. prints the version string to stdout and a message to stderr,
	//   2. writes the ready-signal file so the helper knows bicep is alive, and
	//   3. polls until the release-signal file appears (keeping the process alive).
	//
	// When fake bicep runs with the OLD implementation it inherits the helper's
	// real stderr handle (PIPE_A).  With the new implementation it only gets the
	// private stderr pipe (PIPE_B) that runBicepRaw created, never PIPE_A.
	bicepScript := "" +
		"@echo off\r\n" +
		"echo Bicep CLI version 0.42.1 (abcdef1234)\r\n" +
		"echo fake bicep stderr 1>&2\r\n" +
		"echo.>\"%GO_BICEP_READY_FILE%\"\r\n" +
		":waitloop\r\n" +
		"if exist \"%GO_BICEP_RELEASE_FILE%\" goto release\r\n" +
		"timeout /t 1 /nobreak >nul 2>&1\r\n" +
		"goto waitloop\r\n" +
		":release\r\n"
	bicepPath := filepath.Join(dir, "bicep.cmd")
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepScript), 0o600))

	// Spawn the "rad" helper with stderr piped.
	// The helper runs Version(), waits for fake bicep to start, then calls
	// os.Exit(0) while bicep may still be alive.
	cmd := exec.Command(os.Args[0], "-test.run", "^TestHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(),
		bicepHelperModeEnv+"=rad",
		bicepReadyFileEnv+"="+readyFile,
		bicepReleaseFileEnv+"="+releaseFile,
		BicepEnvVar+"="+bicepPath,
	)

	stderrPipe, err := cmd.StderrPipe()
	require.NoError(t, err)

	require.NoError(t, cmd.Start())

	// Read all of the helper's stderr before calling Wait.
	// Per the StderrPipe contract, Wait closes the pipe, so reads must complete
	// first.  More importantly, this channel receive IS the regression assertion:
	// with the old implementation io.ReadAll blocks forever (fake bicep holds the
	// caller's handle open); with the fix it returns promptly with a nil error
	// once the helper exits.
	type stderrResult struct {
		data []byte
		err  error
	}
	stderrDone := make(chan stderrResult, 1)
	go func() {
		data, err := io.ReadAll(stderrPipe)
		stderrDone <- stderrResult{data, err}
	}()

	select {
	case res := <-stderrDone:
		// nil error means the pipe closed cleanly (not force-closed by Wait).
		require.NoError(t, res.err,
			"stderr pipe must close cleanly; a non-nil error indicates Wait closed the pipe before ReadAll finished")
		require.Contains(t, string(res.data), "rad helper exiting",
			"helper must have written its exit message before closing stderr")
	case <-time.After(10 * time.Second):
		// If we get here, fake bicep is still holding the caller's stderr handle
		// open — the regression is present.
		t.Fatal("stderr pipe did not reach EOF within 10 s: " +
			"regression — fake bicep may be holding the caller's stderr handle open")
	}

	// The helper already exited via os.Exit(0), so Wait returns immediately.
	_ = cmd.Wait()
}
