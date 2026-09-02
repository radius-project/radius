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

package process

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

var hasConsole = func() bool {
	// GetConsoleWindow returns zero for Windows Terminal and other pseudoconsole hosts even when
	// the process has an attached console. GetConsoleCP succeeds for both classic consoles and
	// pseudoconsole hosts, so it preserves interactive child behavior in either environment.
	_, err := windows.GetConsoleCP()
	return err == nil
}

func configure(cmd *exec.Cmd) *exec.Cmd {
	if hasConsole() {
		return cmd
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	return cmd
}
