// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubectl

import (
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
		executableArgs = append(executableArgs, "/c", "kubectl")
	} else {
		executableName = "kubectl"
	}

	executableArgs = append(executableArgs, args...)

	c := exec.Command(executableName, executableArgs...)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	return err
}
