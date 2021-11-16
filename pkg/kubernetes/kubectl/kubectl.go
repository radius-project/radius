// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubectl

import (
	"fmt"
	"os"
	"os/exec"
)

// RunCLICommand runs an kubectl CLI command with stdout and stderr forwarded to this process's output.
func RunCLICommand(args ...string) error {
	var executableName string
	var executableArgs []string

	executableName = "kubectl"

	executableArgs = append(executableArgs, args...)

	c := exec.Command(executableName, executableArgs...)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	return err
}
