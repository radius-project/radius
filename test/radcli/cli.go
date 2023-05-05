// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Contains support for automating the use of the rad CLI
package radcli

import (
	"context"
	"encoding/json"
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

type CLIError struct {
	v1.ErrorResponse
}

func (err *CLIError) Error() string {
	return fmt.Sprintf("code %v: err %v", err.ErrorResponse.Error.Code, err.ErrorResponse.Error.Message)
}

// GetFirstErrorCode function goes down the error chain to find and return the code of the first error in the chain.
func (err *CLIError) GetFirstErrorCode() string {
	var errorCode = err.ErrorResponse.Error.Code

	errorQueue := make([]v1.ErrorDetails, 0)
	errorQueue = append(errorQueue, err.ErrorResponse.Error.Details...)

	for len(errorQueue) > 0 {
		currentErrorDetail := errorQueue[0]
		errorQueue = errorQueue[1:]

		if currentErrorDetail.Code != "OK" {
			errorCode = currentErrorDetail.Code
		}

		if len(currentErrorDetail.Details) > 0 {
			errorQueue = append(errorQueue, currentErrorDetail.Details...)
		}
	}

	return errorCode
}

// Deploy runs the rad deploy command.
func (cli *CLI) Deploy(ctx context.Context, templateFilePath string, application string, parameters ...string) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	args := []string{
		"deploy",
		templateFilePath,
	}

	if application != "" {
		args = append(args, "--application", application)
	}

	for _, parameter := range parameters {
		args = append(args, "--parameters", parameter)
	}

	out, cliErr := cli.RunCommand(ctx, args)
	if cliErr != nil && strings.Contains(out, "Error: {") {
		var errResponse v1.ErrorResponse
		idx := strings.Index(out, "Error: {")
		idxTraceId := strings.Index(out, "TraceId")
		var actualErr string

		if idxTraceId < 0 {
			idxTraceId = len(out)
		}
		actualErr = "{\"error\": " + out[idx+7:idxTraceId-1] + "}"

		if err := json.Unmarshal([]byte(string(actualErr)), &errResponse); err != nil {
			return err
		}

		return &CLIError{ErrorResponse: errResponse}
	}

	return cliErr
}

// Run runs the rad run command. This returns the command output on success or an error.
func (cli *CLI) Run(ctx context.Context, templateFilePath string, applicationName string, parameters ...string) (string, error) {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return "", fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	args := []string{
		"run",
		templateFilePath,
	}

	if applicationName != "" {
		args = append(args, "--application", applicationName)
	}

	for _, parameter := range parameters {
		args = append(args, "--parameters", parameter)
	}

	out, cliErr := cli.RunCommand(ctx, args)
	if cliErr != nil && strings.Contains(out, "Error: {") {
		var errResponse v1.ErrorResponse
		idx := strings.Index(out, "Error: {")
		actualErr := "{\"error\": " + out[idx+7:] + "}"

		if err := json.Unmarshal([]byte(string(actualErr)), &errResponse); err != nil {
			return "", err
		}

		return "", &CLIError{ErrorResponse: errResponse}
	} else if cliErr != nil {
		return "", cliErr
	}

	return out, nil
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

func (cli *CLI) EnvShow(ctx context.Context) (string, error) {
	args := []string{
		"env",
		"show",
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

func (cli *CLI) RecipeList(ctx context.Context, envName string) (string, error) {
	args := []string{
		"recipe",
		"list",
		"--environment", envName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) RecipeRegister(ctx context.Context, envName, recipeName, templatePath, linkType string) (string, error) {
	args := []string{
		"recipe",
		"register",
		recipeName,
		"--environment", envName,
		"--template-path", templatePath,
		"--link-type", linkType,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) RecipeUnregister(ctx context.Context, envName, recipeName, linkType string) (string, error) {
	args := []string{
		"recipe",
		"unregister",
		recipeName,
		"--link-type", linkType,
		"--environment", envName,
	}
	return cli.RunCommand(ctx, args)
}

func (cli *CLI) RecipeShow(ctx context.Context, envName, recipeName string, linkType string) (string, error) {
	args := []string{
		"recipe",
		"show",
		recipeName,
		"--link-type", linkType,
		"--environment", envName,
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

// CreateCommand creates an exec.Cmd that can be used to execute a `rad` CLI command. Most tests should use
// RunCommand, only use CreateCommand if the test you are writing needs more control over the execution of the
// command.
//
// The second return value is the 'heartbeat' func. Tests using this function should start the heartbeat in a
// goroutine to produce logs while the command is running. The third return value is the command description
// which can be used in error messages.
func (cli *CLI) CreateCommand(ctx context.Context, args []string) (*exec.Cmd, func(<-chan struct{}), string) {
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

	heartbeat := func(done <-chan struct{}) {
		cli.heartbeat(description, done)
	}

	return cmd, heartbeat, description
}

// ReportCommandResult can be used in tests to report the result of a command to the test infrastructure. Most
// test should use RunCommand. Only use ReportCommandResult if the test is using CreateCommand to control command
// execution.
func (cli *CLI) ReportCommandResult(ctx context.Context, out string, description string, err error) error {
	// If there's no context error, we know the command completed (or errored).
	for _, line := range strings.Split(out, "\n") {
		cli.T.Logf("[rad] %s", line)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command '%s' timed out: %w", description, err)
	}

	if ctx.Err() == context.Canceled {
		// bubble up errors due to cancellation with their output, and let the caller
		// decide how to handle it.
		return ctx.Err()
	}

	if err != nil {
		return fmt.Errorf("command '%s' had non-zero exit code: %w", description, err)
	}

	return nil
}

// RunCommand executes a command and returns the output (stdout + stderr) as well as an error if the command
// fails. The output is *ALWAYS* returned even if the command fails.
func (cli *CLI) RunCommand(ctx context.Context, args []string) (string, error) {
	cmd, heartbeat, description := cli.CreateCommand(ctx, args)

	// we run a background goroutine to report a heartbeat in the logs while the command
	// is still running. This makes it easy to see what's still in progress if we hit a timeout.
	done := make(chan struct{})
	go heartbeat(done)
	defer func() {
		done <- struct{}{}
		close(done)
	}()

	// Execute the command and get the output.
	out, err := cmd.CombinedOutput()

	return string(out), cli.ReportCommandResult(ctx, string(out), description, err)
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
