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
	"path"
	"strings"
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

const (
	HeartbeatInterval = 10 * time.Second
)

type CLI struct {
	T                *testing.T
	ConfigFilePath   string
	WorkingDirectory string
}

func NewCLI(t *testing.T, configFilePath string) *CLI {
	return &CLI{
		T:              t,
		ConfigFilePath: configFilePath,
	}
}

type CliError struct {
	Message string
	Code    string
}

func (err *CliError) Error() string {
	return fmt.Sprintf("code %v: err %v", err.Code, err.Message)
}

func NewCliError(message string, code string) *CliError {
	err := new(CliError)
	err.Message = message
	err.Code = code
	return err
}

// Deploy runs the rad deploy command.
func (cli *CLI) Deploy(ctx context.Context, templateFilePath string, parameters ...string) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	args := []string{
		"deploy",
		templateFilePath,
	}

	for _, parameter := range parameters {
		args = append(args, "--parameters", parameter)
	}

	out, cliErr := cli.RunCommand(ctx, args)
	if cliErr != nil && strings.Contains(out, "BadRequest") {
		return NewCliError(cliErr.Error(), v1.CodeInvalid)
	}
	return cliErr
}

func (cli *CLI) ApplicationDeploy(ctx context.Context) error {
	command := "application"

	args := []string{
		command,
		"deploy",
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

func (cli *CLI) ApplicationInit(ctx context.Context, applicationName string) error {
	command := "application"

	args := []string{
		command,
		"init",
	}

	if applicationName != "" {
		args = append(args, applicationName)
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

func (cli *CLI) ApplicationShow(ctx context.Context, applicationName string) (string, error) {
	command := "application"

	args := []string{
		command,
		"show",
		"-a", applicationName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) ApplicationList(ctx context.Context) (string, error) {
	command := "application"

	args := []string{
		command,
		"list",
	}
	return cli.RunCommand(ctx, args)
}

// ApplicationDelete deletes the specified application deployed by Radius.
func (cli *CLI) ApplicationDelete(ctx context.Context, applicationName string) error {
	command := "application"

	args := []string{
		command,
		"delete",
		"--yes",
		"-a", applicationName,
	}
	_, err := cli.RunCommand(ctx, args)
	return err
}

func (cli *CLI) EnvStatus(ctx context.Context) (string, error) {
	args := []string{
		"env",
		"status",
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) EnvList(ctx context.Context) (string, error) {
	args := []string{
		"env",
		"list",
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) EnvDelete(ctx context.Context, environmentName string) error {
	args := []string{
		"env",
		"delete",
		"--yes",
		"-e", environmentName,
	}
	_, err := cli.RunCommand(ctx, args)
	return err
}

func (cli *CLI) ResourceShow(ctx context.Context, resourceType string, resourceName string) (string, error) {
	args := []string{
		"resource",
		"show",
		//"-a", applicationName, TODO: apply when application flag (-a) is re-enabled for rad resource show
		resourceType,
		resourceName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) ResourceList(ctx context.Context, applicationName string) (string, error) {
	args := []string{
		"resource",
		"list",
		"containers",
		"-a", applicationName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) ResourceLogs(ctx context.Context, applicationName string, resourceName string) (string, error) {
	args := []string{
		"resource",
		"logs",
		"-a", applicationName,
		"containers",
		resourceName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) ResourceExpose(ctx context.Context, applicationName string, resourceName string, localPort int, remotePort int) (string, error) {
	args := []string{
		"resource",
		"expose",
		"-a", applicationName,
		"containers",
		resourceName,
		"--port", fmt.Sprintf("%d", localPort),
		"--remote-port", fmt.Sprintf("%d", remotePort),
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) Version(ctx context.Context) (string, error) {
	args := []string{
		"version",
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) CliVersion(ctx context.Context) (string, error) {
	args := []string{
		"version",
		"--cli",
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) RunCommand(ctx context.Context, args []string) (string, error) {
	description := "rad " + strings.Join(args, " ")
	args = cli.appendStandardArgs(args)

	radExecutable := "rad"
	if v, found := os.LookupEnv("RAD_PATH"); found {
		radExecutable = path.Join(v, radExecutable)
	}

	cmd := exec.CommandContext(ctx, radExecutable, args...)
	if cli.WorkingDirectory != "" {
		cmd.Dir = cli.WorkingDirectory
	}

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
