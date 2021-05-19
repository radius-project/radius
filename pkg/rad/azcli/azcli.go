// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// RunCLICommand runs an az CLI command with stdout and stderr forwarded to this process's output.
func RunCLICommand(args ...string) error {
	var executableName string
	var executableArgs []string
	if runtime.GOOS == "windows" {
		// Use shell on windows since az is a script not an executable
		executableName = fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir"))
		executableArgs = append(executableArgs, "/c", "az")
	} else {
		executableName = "az"
	}

	executableArgs = append(executableArgs, args...)

	c := exec.Command(executableName, executableArgs...)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	return err
}

// RunCLICommandWithOutput runs an az CLI command with stdout and stderr forwarded to this process's output.
func RunCLICommandWithOutput(args ...string) (string, error) {
	var executableName string
	var executableArgs []string
	if runtime.GOOS == "windows" {
		// Use shell on windows since az is a script not an executable
		executableName = fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir"))
		executableArgs = append(executableArgs, "/c", "az")
	} else {
		executableName = "az"
	}

	executableArgs = append(executableArgs, args...)

	c := exec.Command(executableName, executableArgs...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = os.Stderr
	err := c.Run()
	return out.String(), err
}
