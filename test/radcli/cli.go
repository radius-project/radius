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

	"github.com/Azure/radius/test/validation"
)

const (
	HeartbeatInterval = 10 * time.Second
)

type CLI struct {
	T              *testing.T
	ConfigFilePath string
	Version        validation.AppModelVersion
}

func NewCLI(t *testing.T, configFilePath string, version validation.AppModelVersion) *CLI {
	return &CLI{
		T:              t,
		ConfigFilePath: configFilePath,
		Version:        version,
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
	_, err := cli.RunCommand(ctx, fmt.Sprintf("rad deploy %s", templateFilePath), args)
	return err
}

func (cli *CLI) ApplicationShow(ctx context.Context, applicationName string) (string, error) {
	command := "application"
	if cli.Version == validation.AppModelV3 {
		command = "applicationV3"
	}

	args := []string{
		command,
		"show",
		"-a", applicationName,
	}
	return cli.RunCommand(ctx, fmt.Sprintf("rad application show -a %s", applicationName), args)
}

// ApplicationDelete deletes the specified application deployed by Radius.
func (cli *CLI) ApplicationDelete(ctx context.Context, applicationName string) error {
	command := "application"
	if cli.Version == validation.AppModelV3 {
		command = "applicationV3"
	}

	args := []string{
		command,
		"delete",
		"--yes",
		"-a", applicationName,
	}
	_, err := cli.RunCommand(ctx, fmt.Sprintf("rad application delete -a %s", applicationName), args)
	return err
}

func (cli *CLI) ComponentShow(ctx context.Context, applicationName string, componentName string) (string, error) {
	args := []string{
		"component",
		"show",
		"-a", applicationName,
		componentName,
	}
	return cli.RunCommand(ctx, fmt.Sprintf("rad component show -a %s %s", applicationName, componentName), args)
}

func (cli *CLI) ComponentList(ctx context.Context, applicationName string) (string, error) {
	args := []string{
		"component",
		"list",
		"-a", applicationName,
	}
	return cli.RunCommand(ctx, fmt.Sprintf("rad component list -a %s", applicationName), args)
}

func (cli *CLI) ComponentLogs(ctx context.Context, applicationName string, componentName string) (string, error) {
	args := []string{
		"component",
		"logs",
		"-a", applicationName,
		componentName,
	}
	return cli.RunCommand(ctx, fmt.Sprintf("rad component logs -a %s %s", applicationName, componentName), args)
}

func (cli *CLI) ComponentExpose(ctx context.Context, applicationName string, componentName string, localPort int, remotePort int) (string, error) {
	args := []string{
		"component",
		"expose",
		"-a", applicationName,
		componentName,
		"--port", fmt.Sprintf("%d", localPort),
		"--remote-port", fmt.Sprintf("%d", remotePort),
	}
	return cli.RunCommand(ctx, fmt.Sprintf("rad component expose -a %s %s...", applicationName, componentName), args)
}

func (cli *CLI) RunCommand(ctx context.Context, description string, args []string) (string, error) {
	args = cli.appendStandardArgs(args)

	cmd := exec.CommandContext(ctx, "rad", args...)

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

	// If there's no context error, we know the command completed (or errored).
	for _, line := range strings.Split(string(out), "\n") {
		cli.T.Logf("[rad] %s", line)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command '%s' timed out: %w", description, err)
	}

	if ctx.Err() == context.Canceled {
		// bubble up errors due to cancellation with their output, and let the caller
		// decide how to handle it.
		return string(out), ctx.Err()
	}

	if err != nil {
		return string(out), fmt.Errorf("command '%s' had non-zero exit code: %w", description, err)
	}

	return string(out), nil
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
