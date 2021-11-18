// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubectl

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// RunCLICommand runs an kubectl CLI command with stdout and stderr forwarded to this process's output.
func RunCLICommandSilent(args ...string) error {
	var executableName string
	var executableArgs []string

	executableName = "kubectl"

	executableArgs = append(executableArgs, args...)

	c := exec.Command(executableName, executableArgs...)
	c.Stderr = os.Stderr

	stdout, err := c.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	// asyncronously copy to our buffer, we don't really need to observe
	// errors here since it's copying into memory
	buf := bytes.Buffer{}
	go func() {
		_, _ = io.Copy(&buf, stdout)
	}()

	err = c.Run()
	if err != nil {
		return fmt.Errorf("failed to run command: %w", err)
	}

	bytes, err := io.ReadAll(&buf)
	if err != nil {
		return fmt.Errorf("failed to read kubectl output: %w", err)
	}

	if err != nil {
		return fmt.Errorf("failed executing %q: %w. output: %s", executableName+" "+strings.Join(executableArgs, " "), err, string(bytes))
	}

	return err
}
