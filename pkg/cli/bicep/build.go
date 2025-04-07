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

	"github.com/radius-project/radius/pkg/cli/bicep/tools"
)

// Official regex for semver
// https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
const SemanticVersionRegex = `(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`

// Run rad-bicep with the given args and return the stdout. The stderr
// is not capture but instead redirected to that of the current process.
func runBicepRaw(args ...string) ([]byte, error) {
	if installed, _ := IsBicepInstalled(); !installed {
		return nil, fmt.Errorf("rad-bicep not installed, run \"rad bicep download\" to install")
	}

	binPath, err := tools.GetLocalFilepath(radBicepEnvVar, binaryName)
	if err != nil {
		return nil, fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	// runs 'rad-bicep'
	fullCmd := binPath + " " + strings.Join(args, " ")
	c := exec.Command(binPath, args...)
	c.Stderr = os.Stderr
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}

	err = c.Start()
	if err != nil {
		return nil, fmt.Errorf("failed executing %q: %w", fullCmd, err)
	}

	// copy to our buffer, we don't really need to observe
	// errors here since it's copying into memory
	buf := bytes.Buffer{}
	_, _ = io.Copy(&buf, stdout)

	// Wait() will wait for us to finish draining stderr before returning the exit code
	err = c.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed executing %q: %w", fullCmd, err)
	}

	// read the content
	bytes, err := io.ReadAll(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read rad-bicep output: %w", err)
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
