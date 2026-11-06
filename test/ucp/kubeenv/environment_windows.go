//go:build windows

/*
Copyright 2026 The Radius Authors.

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

package kubeenv

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Serialize discovery and launch so two environments in the same test process
// cannot claim one another's children. Separate package test processes are independent.
var environmentStartMutex sync.Mutex

func startEnvironment(environment *envtest.Environment) (*rest.Config, func() error, error) {
	environmentStartMutex.Lock()
	defer environmentStartMutex.Unlock()

	before, err := childProcessIDs()
	if err != nil {
		return nil, func() error { return nil }, err
	}

	cfg, startErr := environment.Start()
	var paths []string
	if environment.ControlPlane.APIServer != nil {
		paths = append(paths, environment.ControlPlane.APIServer.Path)
	}
	if environment.ControlPlane.Etcd != nil {
		paths = append(paths, environment.ControlPlane.Etcd.Path)
	}
	handles, captureErr := captureNewProcesses(before, paths)

	stop := func() error {
		// controller-runtime uses SIGTERM, which os.Process.Signal does not
		// implement on Windows. Terminate only the children captured for this
		// environment, then let envtest clean up certificates and directories.
		processErr := stopProcesses(handles)
		deadline := time.Now().Add(5 * time.Second)
		for {
			stopErr := environment.Stop()
			if processErr != nil || !errors.Is(stopErr, syscall.EWINDOWS) || time.Now().After(deadline) {
				return errors.Join(processErr, stopErr)
			}
			// The OS handles have signaled, but envtest's Cmd.Wait goroutines
			// may still need to record that the children exited.
			time.Sleep(10 * time.Millisecond)
		}
	}
	return cfg, stop, errors.Join(startErr, captureErr)
}

func childProcessIDs() (children map[uint32]bool, resultErr error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("snapshotting test child processes: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(snapshot)) }()

	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	children = map[uint32]bool{}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID == uint32(os.Getpid()) {
			children[entry.ProcessID] = true
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("enumerating test child processes: %w", err)
	}
	return children, nil
}

func captureNewProcesses(before map[uint32]bool, paths []string) ([]windows.Handle, error) {
	children, err := childProcessIDs()
	if err != nil {
		return nil, err
	}
	var handles []windows.Handle
	var errs []error
	for pid := range children {
		if before[pid] {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			continue // The child exited between the snapshot and OpenProcess.
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("opening test child process %d: %w", pid, err))
			continue
		}

		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		if err != nil {
			errs = append(errs, fmt.Errorf("identifying test child process %d: %w", pid, err))
			errs = append(errs, windows.CloseHandle(handle))
			continue
		}
		image := windows.UTF16ToString(buffer[:size])
		matches := false
		for _, path := range paths {
			if path == "" {
				continue
			}
			executable, err := exec.LookPath(path)
			if err != nil {
				errs = append(errs, fmt.Errorf("resolving envtest executable %q: %w", path, err))
				continue
			}
			executable, err = filepath.Abs(executable)
			if err != nil {
				errs = append(errs, fmt.Errorf("resolving absolute envtest executable %q: %w", path, err))
				continue
			}
			if strings.EqualFold(filepath.Clean(image), filepath.Clean(executable)) {
				matches = true
				break
			}
		}
		if matches {
			// Retaining the handle, rather than just its PID, prevents PID
			// reuse from targeting a different process during cleanup.
			handles = append(handles, handle)
		} else {
			errs = append(errs, windows.CloseHandle(handle))
		}
	}
	return handles, errors.Join(errs...)
}

func stopProcesses(handles []windows.Handle) error {
	var errs []error
	for _, handle := range handles {
		result, err := windows.WaitForSingleObject(handle, 0)
		if err == nil && result == uint32(windows.WAIT_TIMEOUT) {
			err = windows.TerminateProcess(handle, 1)
			if err != nil {
				// Exiting between WaitForSingleObject and TerminateProcess is
				// successful cleanup, not a termination failure.
				result, waitErr := windows.WaitForSingleObject(handle, 0)
				if waitErr == nil && result == windows.WAIT_OBJECT_0 {
					err = nil
				}
			}
			if err == nil {
				result, err = windows.WaitForSingleObject(handle, 10000)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("stopping envtest child process: %w", err))
		} else if result != windows.WAIT_OBJECT_0 {
			errs = append(errs, fmt.Errorf("waiting for envtest child process to exit: wait result %d", result))
		}
		errs = append(errs, windows.CloseHandle(handle))
	}
	return errors.Join(errs...)
}
