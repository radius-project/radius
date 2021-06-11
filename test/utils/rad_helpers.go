// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// RunRadDeployCommand runs rad deploy command and times out after specified duration
func RunRadDeployCommand(templateFilePath, configFilePath string, ctx context.Context) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	// Create the command with our context
	var cmd *exec.Cmd
	if configFilePath == "" {
		cmd = exec.CommandContext(ctx, "rad", "deploy", templateFilePath)
	} else {
		if _, err := os.Stat(configFilePath); err != nil {
			return fmt.Errorf("could not find radius config file: %s - %w", configFilePath, err)
		}

		cmd = exec.CommandContext(ctx, "rad", "deploy", templateFilePath, "--config", configFilePath)
	}
	err := RunCommand(ctx, cmd)
	return err
}

// RunRadApplicationDeleteCommand deletes all applications deployed by Radius in the specified resource group
func RunRadApplicationDeleteCommand(applicationName, configFilePath string, ctx context.Context) error {
	// Create the command with our context
	var cmd *exec.Cmd
	if configFilePath == "" {
		cmd = exec.CommandContext(ctx, "rad", "application", "delete", "--yes", "-a", applicationName)
	} else {
		if _, err := os.Stat(configFilePath); err != nil {
			return fmt.Errorf("error deploying template using configfile: %s - %w", configFilePath, err)
		}

		cmd = exec.CommandContext(ctx, "rad", "application", "delete", "--yes", "-a", applicationName, "--config", configFilePath)
	}

	err := RunCommand(ctx, cmd)
	return err
}

// RunCommand runs a shell command
func RunCommand(ctx context.Context, cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Println("command timed out")
		return ctx.Err()
	}

	// If there's no context error, we know the command completed (or errored).
	fmt.Println("Output:", string(out))
	if err != nil {
		fmt.Println("non-zero exit code:", err.Error())
	}

	return err
}

func GetContext(t *testing.T) (context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	deadline, ok := t.Deadline()
	if ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	return ctx, cancel
}
