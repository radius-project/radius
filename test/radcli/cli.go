// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Contains support for automating the use of the rad CLI
package radcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	HeartbeatInterval = 10 * time.Second
)

type CLI struct {
	T              *testing.T
	ConfigFilePath string
}

func NewCLI(t *testing.T, configFilePath string) *CLI {
	return &CLI{
		T:              t,
		ConfigFilePath: configFilePath,
	}
}

// Deploy runs the rad deploy command.
func (cli *CLI) Deploy(ctx context.Context, templateFilePath string) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	args := []string{
		"deploy",
		templateFilePath,
	}
	args = cli.appendStandardArgs(args)

	cmd := exec.CommandContext(ctx, "rad", args...)
	err := cli.runCommand(ctx, fmt.Sprintf("rad deploy %s", templateFilePath), cmd)
	return err
}

// ApplicationDelete deletes the specified application deployed by Radius.
func (cli *CLI) ApplicationDelete(ctx context.Context, applicationName string) error {
	args := []string{
		"application",
		"delete",
		"--yes",
		"-a", applicationName,
	}
	args = cli.appendStandardArgs(args)

	cmd := exec.CommandContext(ctx, "rad", args...)
	err := cli.runCommand(ctx, fmt.Sprintf("rad delete -a %s", applicationName), cmd)
	return err
}

func (cli *CLI) runCommand(ctx context.Context, description string, cmd *exec.Cmd) error {
	// we run a background goroutine to report a heartbeat in the logs while the command
	// is still running. This makes it easy to see what's still in progress if we hit a timeout.
	done := make(chan struct{})
	go func() {
		cli.heartbeat(description, done)
	}()
	defer func() {
		done <- struct{}{}
	}()

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command '%s' timed out: %w", description, err)
	}

	// If there's no context error, we know the command completed (or errored).
	for _, line := range strings.Split(string(out), "\n") {
		cli.T.Logf("[rad] %s", line)
	}

	if err != nil {
		return fmt.Errorf("command '%s' had non-zero exit code: %w", description, err)
	}

	return err
}

func (cli *CLI) appendStandardArgs(args []string) []string {
	if cli.ConfigFilePath == "" {
		return args
	}

	return append(args, "--config", cli.ConfigFilePath)
}

func (cli *CLI) heartbeat(description string, done <-chan struct{}) {
	start := time.Now()
	for {
		select {
		case <-time.After(HeartbeatInterval):
			cli.T.Logf("[heartbeat] command %s is still running after %s", description, time.Since(start))
		case <-done:
			return
		}
	}
}
