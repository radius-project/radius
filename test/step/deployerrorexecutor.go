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

package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/radcli"
)

var _ Executor = (*DeployErrorExecutor)(nil)

type DeployErrorExecutor struct {
	Description        string
	Template           string
	Parameters         []string
	ExpectedErrorCode  string
	ExpectedInnerError []string

	// Application sets the `--application` command-line parameter. This is needed in cases where
	// the application is not defined in bicep.
	Application string

	// Environment sets the `--environment` command-line parameter. This is needed in cases where
	// the environment is not defined in bicep.
	Environment string
}

// # Function Explanation
//
// NewDeployErrorExecutor creates a new DeployErrorExecutor instance with the given template, error code and parameters.
func NewDeployErrorExecutor(template string, errCode string, innerError []string, parameters ...string) *DeployErrorExecutor {
	return &DeployErrorExecutor{
		Description:        fmt.Sprintf("deploy %s", template),
		Template:           template,
		Parameters:         parameters,
		ExpectedErrorCode:  errCode,
		ExpectedInnerError: innerError,
	}
}

// # Function Explanation
//
// WithApplication sets the application name for the DeployErrorExecutor instance and returns the instance.
func (d *DeployErrorExecutor) WithApplication(application string) *DeployErrorExecutor {
	d.Application = application
	return d
}

// WithEnvironment sets the environment name for the DeployExecutor instance and returns the same instance.
func (d *DeployErrorExecutor) WithEnvironment(environment string) *DeployErrorExecutor {
	d.Environment = environment
	return d
}

// # Function Explanation
//
// GetDescription returns the Description field of the DeployErrorExecutor instance.
func (d *DeployErrorExecutor) GetDescription() string {
	return d.Description
}

// # Function Explanation
//
// Execute deploys an application from a template file and checks that the deployment fails with the expected error code.
func (d *DeployErrorExecutor) Execute(ctx context.Context, t *testing.T, options test.TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath, d.Environment, d.Application, d.Parameters...)
	require.Error(t, err, "deployment %s succeeded when it should have failed", d.Description)

	var cliErr *radcli.CLIError
	require.ErrorAs(t, err, &cliErr, "error should be a CLIError and it was not")
	require.Equal(t, d.ExpectedErrorCode, cliErr.GetFirstErrorCode())

	if len(d.ExpectedInnerError) > 0 {
		unpackErrorAndMatch(err, d.ExpectedInnerError)
	}

	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
