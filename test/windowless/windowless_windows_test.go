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

package windowless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/process"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

const (
	testHelperEnv          = "RADIUS_TEST_HELPER"
	testHelperParent       = "automation-parent"
	testHelperBicep        = "bicep"
	testRadBinaryEnv       = "RADIUS_TEST_RAD_BINARY"
	testBicepModeEnv       = "RADIUS_TEST_BICEP_MODE"
	testBicepStatusEnv     = "RADIUS_TEST_BICEP_STATUS"
	testBicepModeHang      = "hang"
	testProcessTimeout     = 20 * time.Second
	testJobExitCode        = 125
	testBicepVersion       = "0.42.1"
	testBicepStderr        = "Bicep diagnostic from test"
	testStatusPollInterval = 25 * time.Millisecond
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	freeConsole      = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")
	radBinaryPath    string
)

func TestMain(m *testing.M) {
	switch os.Getenv(testHelperEnv) {
	case testHelperParent:
		os.Exit(runAutomationParent()) //nolint:forbidigo // Test helper subprocess must return the rad exit code.
	case testHelperBicep:
		os.Exit(runFakeBicep()) //nolint:forbidigo // Test helper subprocess must behave as the Bicep executable.
	}

	tempDir, err := os.MkdirTemp("", "radius-windowless-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) //nolint:forbidigo // TestMain cannot continue without a temporary build directory.
	}
	radBinaryPath = filepath.Join(tempDir, "rad.exe")
	if err := buildRad(radBinaryPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(tempDir)
		os.Exit(1) //nolint:forbidigo // TestMain must report a failed integration-test build.
	}

	code := m.Run()
	_ = os.RemoveAll(tempDir)
	os.Exit(code) //nolint:forbidigo // TestMain must return the test suite exit code.
}

func TestRadVersion_NonDetachedWindowlessProcessCompletesInsideJob(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "bicep-status.txt")
	ctx, cancel := context.WithTimeout(context.Background(), testProcessTimeout)
	defer cancel()

	cmd := newAutomationParentCommand(ctx, statusPath, "")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		status, _ := os.ReadFile(statusPath)
		require.NoErrorf(t, err, "stderr: %s\nBicep status: %s\ncontext: %v", stderr.String(), status, ctx.Err())
	}

	var result struct {
		Bicep string `json:"bicep"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), stdout.String())
	require.Equal(t, testBicepVersion, result.Bicep)
	require.Contains(t, stderr.String(), testBicepStderr)

	status := readBicepStatus(t, statusPath)
	require.Equal(t, "false", status["console"], "Bicep must not have an attached console")
}

func TestRadVersion_CancellationTerminatesJobProcessTree(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "bicep-status.txt")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := newAutomationParentCommand(ctx, statusPath, testBicepModeHang)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())

	status := waitForBicepStatus(t, statusPath, testProcessTimeout)
	pid, err := strconv.ParseUint(status["pid"], 10, 32)
	require.NoError(t, err)
	require.Equal(t, "false", status["console"], "Bicep must not have an attached console")

	processHandle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = windows.TerminateProcess(processHandle, testJobExitCode)
		_ = windows.CloseHandle(processHandle)
	})

	cancel()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case err := <-waitDone:
		require.Error(t, err)
	case <-time.After(testProcessTimeout):
		t.Fatal("automation parent did not exit after cancellation")
	}

	waitResult, err := windows.WaitForSingleObject(processHandle, uint32(testProcessTimeout.Milliseconds()))
	require.NoError(t, err)
	require.Equal(t, uint32(windows.WAIT_OBJECT_0), waitResult, "Bicep outlived the canceled automation parent")
}

func buildRad(outputPath string) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to locate repository root")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/rad")
	cmd.Dir = repositoryRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build rad: %w: %s", err, output)
	}
	return nil
}

func newAutomationParentCommand(ctx context.Context, statusPath string, bicepMode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(),
		testHelperEnv+"="+testHelperParent,
		testRadBinaryEnv+"="+radBinaryPath,
		testBicepModeEnv+"="+bicepMode,
		testBicepStatusEnv+"="+statusPath,
	)
	return cmd
}

func runAutomationParent() int {
	if result, _, err := freeConsole.Call(); result == 0 {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	job, err := createKillOnCloseJob()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Keep the only job handle open for the helper lifetime. Its automatic close when the helper
	// exits is what terminates rad and every descendant during the cancellation test.

	cmd := process.Command(os.Getenv(testRadBinaryEnv), "version", "--cli", "--output", "json")
	cmd.Env = append(os.Environ(),
		testHelperEnv+"="+testHelperBicep,
		bicep.BicepEnvVar+"="+os.Args[0],
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	stdoutDone := copyPipe(stdout, os.Stdout)
	stderrDone := copyPipe(stderr, os.Stderr)
	stdoutResult := <-stdoutDone
	stderrResult := <-stderrDone
	waitErr := cmd.Wait()

	if stdoutResult.err != nil || stderrResult.err != nil {
		fmt.Fprintf(os.Stderr, "failed to read rad output: %v %v\n", stdoutResult.err, stderrResult.err)
		return 1
	}
	if waitErr != nil {
		fmt.Fprintln(os.Stderr, waitErr)
		return 1
	}
	return 0
}

func createKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

type pipeResult struct {
	content []byte
	err     error
}

func copyPipe(reader io.Reader, writer io.Writer) <-chan pipeResult {
	done := make(chan pipeResult, 1)
	go func() {
		var content bytes.Buffer
		_, err := io.Copy(io.MultiWriter(&content, writer), reader)
		done <- pipeResult{content: content.Bytes(), err: err}
	}()
	return done
}

func runFakeBicep() int {
	consoleWindow, _, _ := getConsoleWindow.Call()
	status := fmt.Sprintf("pid=%d\nconsole=%t\n", os.Getpid(), consoleWindow != 0)
	if err := os.WriteFile(os.Getenv(testBicepStatusEnv), []byte(status), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if os.Getenv(testBicepModeEnv) == testBicepModeHang {
		for {
			time.Sleep(time.Hour)
		}
	}

	fmt.Fprintln(os.Stderr, testBicepStderr)
	fmt.Printf("Bicep CLI version %s (test)\n", testBicepVersion)
	return 0
}

func readBicepStatus(t *testing.T, path string) map[string]string {
	t.Helper()
	return waitForBicepStatus(t, path, time.Second)
}

func waitForBicepStatus(t *testing.T, path string, timeout time.Duration) map[string]string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			status := map[string]string{}
			for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
				key, value, ok := strings.Cut(line, "=")
				if ok {
					status[key] = value
				}
			}
			if status["pid"] != "" && status["console"] != "" {
				return status
			}
		}
		if !os.IsNotExist(err) {
			require.NoError(t, err)
		}
		time.Sleep(testStatusPollInterval)
	}
	t.Fatalf("Bicep did not write status within %s", timeout)
	return nil
}
