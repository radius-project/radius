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
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/radius-project/radius/pkg/cli/bicep/tools"
)

// Official regex for semver
// https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
const SemanticVersionRegex = `(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`

// Run bicep with the given args and return the stdout. The stderr
// is not capture but instead redirected to that of the current process.
func runBicepRaw(args ...string) ([]byte, error) {
	if installed, _ := IsBicepInstalled(); !installed {
		return nil, fmt.Errorf("bicep not installed, run \"rad bicep download\" to install")
	}

	binPath, err := tools.GetLocalFilepath(BicepEnvVar, binaryName)
	if err != nil {
		return nil, fmt.Errorf("failed to find bicep: %w", err)
	}

	// runs 'bicep'
	fullCmd := binPath + " " + strings.Join(args, " ")
	c := exec.Command(binPath, args...)

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Route bicep's stderr through a pipe we own instead of handing it os.Stderr
	// directly. If bicep inherits the parent process's real stderr handle, a caller
	// that launched rad with a piped stderr won't observe that pipe close until
	// bicep also exits -- which hangs stderr stream-close waits on Windows. Copying
	// through our own pipe keeps the 'stream bicep stderr to ours' behavior without
	// leaking the caller's handle to the grandchild.
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}

	if err = c.Start(); err != nil {
		return nil, fmt.Errorf("failed executing %q: %w", fullCmd, err)
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = io.Copy(os.Stderr, stderr)
	})

	// copy to our buffer, we don't really need to observe
	// errors here since it's copying into memory
	buf := bytes.Buffer{}
	_, _ = io.Copy(&buf, stdout)

	// Ensure stderr is fully drained before Wait() closes the pipe.
	wg.Wait()
	if err = c.Wait(); err != nil {
		return nil, fmt.Errorf("failed executing %q: %w", fullCmd, err)
	}

	// read the content
	bytes, err := io.ReadAll(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read bicep output: %w", err)
	}

	return bytes, nil
}

// Version() attempts to retrieve the version of Bicep by running the command "--version" and returns the version as a
// string, or an error message if an error occurs.
func Version() string {
	bytes, err := runBicepRaw("--version")
	if err != nil {
		return fmt.Sprintf("unknown (%s)", err)
	}

	version := regexp.MustCompile(SemanticVersionRegex).FindString(string(bytes))
	if version == "" {
		return fmt.Sprintf("unknown (failed to parse bicep version from %q)", string(bytes))
	}
	return version
}
