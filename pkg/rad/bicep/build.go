// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Build the provided `.bicep` file and returns the deployment template.
func Build(filePath string) (string, error) {
	filepath, err := GetLocalBicepFilepath()
	if err != nil {
		return "", fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	// runs 'rad-bicep build' on the file
	//
	// rad-bicep is being told to output the template to stdout and we will capture it
	// rad-bicep will output compilation errors to stderr which will go to the user's console
	c := exec.Command(filepath, "build", "--stdout", filePath)
	c.Stderr = os.Stderr
	stdout, err := c.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create pipe: %w", err)
	}

	err = c.Start()
	if err != nil {
		return "", fmt.Errorf("rad-bicep build failed: %w", err)
	}

	// asyncronously copy to our buffer, we don't really need to observe
	// errors here since it's copying into memory
	buf := bytes.Buffer{}
	go func() {
		_, _ = io.Copy(&buf, stdout)
	}()

	// wait will wait for us to finish draining stderr before returning the exit code
	err = c.Wait()
	if err != nil {
		return "", fmt.Errorf("rad-bicep build failed: %w", err)
	}

	// read the content
	bytes, err := io.ReadAll(&buf)
	if err != nil {
		return "", fmt.Errorf("failed to read rad-bicep output: %w", err)
	}

	return string(bytes), err
}
