// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const (
	SternToolName        = "stern"
	SternNotFoundMessage = "The tool %s was not found on the system PATH. %s can stream logs for your application. See %s for installation instructions."
	SternInstallerURL    = "https://github.com/stern/stern#installation"
)

func SternStart(ctx context.Context, context string, namespace string, application string) error {
	exe := GetExecutableName(SternToolName)
	_, err := exec.LookPath(exe)
	if errors.Is(err, exec.ErrNotFound) {
		return ErrToolNotFound{
			Tool:    SternToolName,
			Message: fmt.Sprintf(SternNotFoundMessage, SternToolName, SternToolName, SternInstallerURL),
		}
	}

	args := []string{
		"--context", context,
		"--namespace", application,
		"--selector", fmt.Sprintf("radius.dev/application=%s", application),

		// Dapr sidecars are especially noisy, and usually not relevant.
		"--exclude-container", "daprd",
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Start()
	if err != nil {
		return err
	}

	// Don't block after we successfully start the process. The process will be closed
	// automatically once we cancel.
	return nil
}
