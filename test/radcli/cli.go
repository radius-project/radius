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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const (
	HeartbeatInterval = 10 * time.Second
)

type CLI struct {
	T                *testing.T
	ConfigFilePath   string
	WorkingDirectory string
}

// NewCLI creates a new CLI instance with the given testing.T and config file path.
func NewCLI(t *testing.T, configFilePath string) *CLI {
	return &CLI{
		T:              t,
		ConfigFilePath: configFilePath,
	}
}

type CLIError struct {
	v1.ErrorResponse
}

// CLIError.Error returns a string containing the error code and message of the error response.
func (err *CLIError) Error() string {
	return fmt.Sprintf("code %v: err %v", err.ErrorResponse.Error.Code, err.ErrorResponse.Error.Message)
}

// GetFirstErrorCode function goes down the error chain to find and return the code of the first error in the chain.
func (err *CLIError) GetFirstErrorCode() string {
	var errorCode = err.ErrorResponse.Error.Code

	errorQueue := make([]*v1.ErrorDetails, 0)
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

// Deploy runs the rad deploy command. It checks if the template file path exists and runs the command with the
// given parameters, returning an error if the command fails.
func (cli *CLI) Deploy(ctx context.Context, templateFilePath string, environment string, application string, parameters ...string) error {
	return cli.deployInternal(ctx, templateFilePath, environment, application, "", parameters...)
}

// DeployWithGroup runs the rad deploy command with a specific resource group. It checks if the template file path exists
// and runs the command with the given parameters, returning an error if the command fails.
func (cli *CLI) DeployWithGroup(ctx context.Context, templateFilePath string, environment string, application string, group string, parameters ...string) error {
	return cli.deployInternal(ctx, templateFilePath, environment, application, group, parameters...)
}

// deployInternal is the internal implementation for deploy commands with optional group support.
func (cli *CLI) deployInternal(ctx context.Context, templateFilePath string, environment string, application string, group string, parameters ...string) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		return fmt.Errorf("could not find template file: %s - %w", templateFilePath, err)
	}

	args := []string{
		"deploy",
		templateFilePath,
	}

	if environment != "" {
		args = append(args, "--environment", environment)
	}

	if application != "" {
		args = append(args, "--application", application)
	}

	if group != "" {
		args = append(args, "--group", group)
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

// ShowOptions provides flexible configuration for show commands
type ShowOptions struct {
	Group       string // The resource group name
	Workspace   string // The workspace name
	Output      string // Output format (json, table, plain-text)
	Application string // Application name (for resource show)
}

// DeleteOptions provides configuration for delete commands
type DeleteOptions struct {
	Group     string // Resource group name (--group)
	Workspace string // Workspace name (--workspace)
	Confirm   bool   // Skip confirmation prompt (--yes)
	Output    string // Output format (--output)
}

// CreateOptions provides configuration for create commands
type CreateOptions struct {
	Group       string // Resource group name (--group)
	Workspace   string // Workspace name (--workspace)
	Environment string // Environment name (--environment)
	Namespace   string // Kubernetes namespace (--namespace, for env create)
	Context     string // Kubernetes context (--context, for workspace create)
	Force       bool   // Overwrite if exists (--force, for workspace create)
	Output      string // Output format (--output)
}

// ApplicationShow returns the output of running the "application show" command with flexible options.
// The options parameter is optional and allows specifying group, workspace, and output format.
func (cli *CLI) ApplicationShow(ctx context.Context, applicationName string, opts ...ShowOptions) (string, error) {
	args := []string{
		"application",
		"show",
	}

	if applicationName != "" {
		args = append(args, "-a", applicationName)
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	}

	return cli.RunCommand(ctx, args)
}

// EnvShow returns the output of running the "env show" command with flexible options.
// The options parameter is optional and allows specifying group, workspace, and output format.
func (cli *CLI) EnvShow(ctx context.Context, environmentName string, opts ...ShowOptions) (string, error) {
	args := []string{
		"env",
		"show",
	}

	if environmentName != "" {
		args = append(args, "-e", environmentName)
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	}

	return cli.RunCommand(ctx, args)
}

// ApplicationDelete deletes the specified application deployed by Radius and returns an error if one occurs.
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

// EnvCreate runs the command to create an environment with the given name.
// The options parameter is optional and allows specifying namespace, group, workspace, and output format.
func (cli *CLI) EnvCreate(ctx context.Context, environmentName string, opts ...CreateOptions) error {
	args := []string{
		"env",
		"create",
		environmentName,
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Namespace != "" {
			args = append(args, "--namespace", opt.Namespace)
		}
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

// EnvDelete runs the command to delete an environment with the given name and returns an error if the command fails.
// The options parameter is optional and allows specifying group, workspace, and confirmation bypass.
func (cli *CLI) EnvDelete(ctx context.Context, environmentName string, opts ...DeleteOptions) error {
	args := []string{
		"env",
		"delete",
		"-e", environmentName,
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Confirm {
			args = append(args, "--yes")
		}
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	} else {
		// Default to --yes for backward compatibility
		args = append(args, "--yes")
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

// ResourceShow runs the rad resource show command with flexible options.
// The options parameter is optional and allows specifying group, workspace, output format, and application.
func (cli *CLI) ResourceShow(ctx context.Context, resourceType string, resourceName string, opts ...ShowOptions) (string, error) {
	args := []string{
		"resource",
		"show",
		resourceType,
		resourceName,
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
		if opt.Application != "" {
			args = append(args, "--application", opt.Application)
		}
	}

	return cli.RunCommand(ctx, args)
}

// ResourceList runs the "resource list containers" command with the given application name and returns the output as a
// string, returning an error if the command fails.
func (cli *CLI) ResourceList(ctx context.Context, applicationName string) (string, error) {
	args := []string{
		"resource",
		"list",
		"Applications.Core/containers",
		"-a", applicationName,
	}
	return cli.RunCommand(ctx, args)
}

// ResourceListInResourceGroup runs the "resource list --group" command to list all resources in a specific resource group
// and returns the output as a string, returning an error if the command fails.
func (cli *CLI) ResourceListInResourceGroup(ctx context.Context, groupName string) (string, error) {
	args := []string{
		"resource",
		"list",
		"--group", groupName,
	}
	return cli.RunCommand(ctx, args)
}

// GroupCreate creates a resource group with the given name.
// The options parameter is optional and allows specifying workspace and output format.
func (cli *CLI) GroupCreate(ctx context.Context, groupName string, opts ...CreateOptions) error {
	args := []string{
		"group",
		"create",
		groupName,
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

// GroupDelete deletes a resource group with the given name. If confirm is true, it will pass the --yes flag
// to skip confirmation prompts. Returns an error if the command fails.
func (cli *CLI) GroupDelete(ctx context.Context, groupName string, opts ...DeleteOptions) error {
	args := []string{
		"group",
		"delete",
		groupName,
	}

	// Apply options if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Confirm {
			args = append(args, "--yes")
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

// GroupList lists all resource groups and returns the output as a string, returning an error if the command fails.
func (cli *CLI) GroupList(ctx context.Context) (string, error) {
	args := []string{
		"group",
		"list",
	}
	return cli.RunCommand(ctx, args)
}

// GroupShow shows details of a specific resource group and returns the output as a string,
// returning an error if the command fails.
func (cli *CLI) GroupShow(ctx context.Context, groupName string) (string, error) {
	args := []string{
		"group",
		"show",
		groupName,
	}
	return cli.RunCommand(ctx, args)
}

// ResourceLogs runs the CLI command to get the logs of a resource in an application.
func (cli *CLI) ResourceLogs(ctx context.Context, applicationName string, resourceName string) (string, error) {
	args := []string{
		"resource",
		"logs",
		"-a", applicationName,
		"Applications.Core/containers",
		resourceName,
	}
	return cli.RunCommand(ctx, args)
}

// ResourceExpose runs a command to expose a resource from an application on a given port.
func (cli *CLI) ResourceExpose(ctx context.Context, applicationName string, resourceName string, localPort int, remotePort int) (string, error) {
	args := []string{
		"resource",
		"expose",
		"-a", applicationName,
		"Applications.Core/containers",
		resourceName,
		"--port", fmt.Sprintf("%d", localPort),
		"--remote-port", fmt.Sprintf("%d", remotePort),
	}
	return cli.RunCommand(ctx, args)
}

// RecipeList runs the "recipe list" command with the given environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) RecipeList(ctx context.Context, envName string) (string, error) {
	args := []string{
		"recipe",
		"list",
		"--environment", envName,
	}
	return cli.RunCommand(ctx, args)
}

// RecipePackList runs the "recipe-pack list" command with the given environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) RecipePackList(ctx context.Context, groupName string) (string, error) {
	args := []string{
		"recipe-pack",
		"list",
	}
	if groupName != "" {
		args = append(args, "--group", groupName)
	}
	return cli.RunCommand(ctx, args)
}

// RecipePackShow runs the "recipe-pack show" command with the given recipe pack name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) RecipePackShow(ctx context.Context, recipepackName, groupName string) (string, error) {
	args := []string{
		"recipe-pack",
		"show",
		recipepackName,
	}
	if groupName != "" {
		args = append(args, "--group", groupName)
	}
	return cli.RunCommand(ctx, args)
}

// EnvironmentCreatePreview runs the "env create" command for the specified environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) EnvironmentCreatePreview(ctx context.Context, environmentName, groupName string) (string, error) {
	args := []string{
		"env",
		"create",
		environmentName,
		"--preview",
	}

	if groupName != "" {
		args = append(args, "--group", groupName)
	}

	return cli.RunCommand(ctx, args)
}

// EnvironmentListPreview runs the "env list" command and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) EnvironmentListPreview(ctx context.Context, groupName string) (string, error) {
	args := []string{
		"env",
		"list",
		"--preview",
	}

	if groupName != "" {
		args = append(args, "--group", groupName)
	}

	return cli.RunCommand(ctx, args)
}

// EnvironmentUpdatePreview runs the "env update" command for the specified environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) EnvironmentUpdatePreview(ctx context.Context, environmentName, groupName, recipepack string) (string, error) {
	args := []string{
		"env",
		"update",
		environmentName,
		"--recipe-packs",
		recipepack,
		"--preview",
	}

	if groupName != "" {
		args = append(args, "--group", groupName)
	}

	return cli.RunCommand(ctx, args)
}

// EnvironmentShowPreview runs the "env show" command for the specified environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) EnvironmentShowPreview(ctx context.Context, environmentName, groupName string) (string, error) {
	args := []string{
		"env",
		"show",
		environmentName,
		"--preview",
	}

	if groupName != "" {
		args = append(args, "--group", groupName)
	}

	return cli.RunCommand(ctx, args)
}

// EnvironmentShowPreview runs the "env show" command for the specified environment name and returns the output as a string, returning
// an error if the command fails.
func (cli *CLI) EnvironmentDeletePreview(ctx context.Context, environmentName, groupName string) (string, error) {
	args := []string{
		"env",
		"delete",
		environmentName,
		"--preview",
		"--yes",
	}

	if groupName != "" {
		args = append(args, "--group", groupName)
	}

	return cli.RunCommand(ctx, args)
}

// RecipePackDelete runs the "recipe-pack delete" command for the specified recipe pack name.
// The options parameter is optional and allows specifying group, workspace, and confirmation bypass.
func (cli *CLI) RecipePackDelete(ctx context.Context, recipepackName string, opts ...DeleteOptions) error {
	args := []string{
		"recipe-pack",
		"delete",
		recipepackName,
	}

	if len(opts) > 0 {
		opt := opts[0]
		if opt.Confirm {
			args = append(args, "--yes")
		}
		if opt.Group != "" {
			args = append(args, "--group", opt.Group)
		}
		if opt.Workspace != "" {
			args = append(args, "--workspace", opt.Workspace)
		}
		if opt.Output != "" {
			args = append(args, "--output", opt.Output)
		}
	} else {
		args = append(args, "--yes")
	}

	_, err := cli.RunCommand(ctx, args)
	return err
}

// RecipeRegister runs a command to register a recipe with the given environment, template kind, template path and
// resource type, and returns the output string or an error.
func (cli *CLI) RecipeRegister(ctx context.Context, envName, recipeName, templateKind, templatePath, resourceType string, plainHTTP bool) (string, error) {
	args := []string{
		"recipe",
		"register",
		recipeName,
		"--environment", envName,
		"--template-kind", templateKind,
		"--template-path", templatePath,
		"--resource-type", resourceType,
	}
	if plainHTTP {
		args = append(args, "--plain-http")
	}
	return cli.RunCommand(ctx, args)
}

// RecipeUnregister runs a command to unregister a recipe from an environment, given the recipe name and resource type.
// It returns a string and an error if the command fails.
func (cli *CLI) RecipeUnregister(ctx context.Context, envName, recipeName, resourceType string) (string, error) {
	args := []string{
		"recipe",
		"unregister",
		recipeName,
		"--resource-type", resourceType,
		"--environment", envName,
	}
	return cli.RunCommand(ctx, args)
}

// RecipeShow runs a command to show a recipe with the given environment name, recipe name and resource type, and returns the
// output string or an error.
func (cli *CLI) RecipeShow(ctx context.Context, envName, recipeName string, resourceType string) (string, error) {
	args := []string{
		"recipe",
		"show",
		recipeName,
		"--resource-type", resourceType,
		"--environment", envName,
	}
	return cli.RunCommand(ctx, args)
}

// BicepPublish runs the bicep publish command with the given file and target, and returns the output string or an error if
// the command fails.
func (cli *CLI) BicepPublish(ctx context.Context, file, target string) (string, error) {
	args := []string{
		"bicep",
		"publish",
		"--file",
		file,
		"--target",
		target,
	}
	return cli.RunCommand(ctx, args)
}

// ResourceProviderCreate runs a command to create or update a resource provider and it's associated resource types from a manifest file.
// It returns the output string or an error if the command fails.
func (cli *CLI) ResourceProviderCreate(ctx context.Context, manifestFilePath string) (string, error) {
	args := []string{
		"resource-provider",
		"create",
		"--from-file",
		manifestFilePath,
	}
	return cli.RunCommand(ctx, args)
}

// ResourceTypeCreate runs a command to create or update a resource type.
// It returns the output string or an error if the command fails.
func (cli *CLI) ResourceTypeCreate(ctx context.Context, resourceTypeName string, manifestFilePath string) (string, error) {
	args := []string{
		"resource-type",
		"create",
		"--from-file",
		manifestFilePath,
	}

	if resourceTypeName != "" {
		args = append(args, resourceTypeName)
	}

	return cli.RunCommand(ctx, args)
}

// Version runs the version command and returns the output as a string, or an error if the command fails.
func (cli *CLI) Version(ctx context.Context) (string, error) {
	args := []string{
		"version",
	}
	return cli.RunCommand(ctx, args)
}

// CliVersion retrieves the version of the CLI by running the "version --cli" command.
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
