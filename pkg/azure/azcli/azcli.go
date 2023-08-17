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

package azcli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// RunCLICommand runs the Azure CLI command based on the OS type and returns an error if the command fails.
// It forwards the stdout and stderr to this process's output.
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
