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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/radcli"
)

var _ Executor = (*DeployErrorExecutor)(nil)

type DeployErrorExecutor struct {
	Description string
	Template    string
	Parameters  []string

	// ValidateError is a function that can be used to validate the error when it occurs.
	ValidateError func(*testing.T, *radcli.CLIError)

	// Application sets the `--application` command-line parameter. This is needed in cases where
	// the application is not defined in bicep.
	Application string

	// Environment sets the `--environment` command-line parameter. This is needed in cases where
	// the environment is not defined in bicep.
	Environment string
}

// DeploymentErrorDetail describes an error that can be matched against the output.
type DeploymentErrorDetail struct {
	// The error code to match.
	Code string

	// The message to match. If provided, this will be matched against a substring of the error.
	MessageContains string

	// The details to match. If provided, this will be matched against the details of the error.
	Details []DeploymentErrorDetail
}

// NewDeployErrorExecutor creates a new DeployErrorExecutor instance with the given template, error code and parameters.
func NewDeployErrorExecutor(template string, validateError func(*testing.T, *radcli.CLIError), parameters ...string) *DeployErrorExecutor {
	return &DeployErrorExecutor{
		Description:   fmt.Sprintf("deploy %s", template),
		Template:      template,
		ValidateError: validateError,
		Parameters:    parameters,
	}
}

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

// GetDescription returns the Description field of the DeployErrorExecutor instance.
func (d *DeployErrorExecutor) GetDescription() string {
	return d.Description
}

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
	if d.ValidateError != nil {
		d.ValidateError(t, err.(*radcli.CLIError))
	}

	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

func (detail DeploymentErrorDetail) Matches(candidate v1.ErrorDetails) bool {
	// Successful deployment of this resource. Skip.
	if candidate.Code == "OK" {
		return false
	}

	if candidate.Code != detail.Code {
		return false
	}

	if detail.MessageContains != "" && !strings.Contains(candidate.Message, detail.MessageContains) {
		return false
	}

	// Details can match recursively.
	if len(detail.Details) > 0 {
		for _, subDetail := range detail.Details {
			matched := false
			for _, candidateSubDetail := range candidate.Details {
				if subDetail.Matches(candidateSubDetail) {
					matched = true
					break
				}
			}

			if !matched {
				return false
			}
		}
	}

	return true
}

// ValidateCode reports success if the error code matches the expected code.
func ValidateCode(code string) func(*testing.T, *radcli.CLIError) {
	return func(t *testing.T, err *radcli.CLIError) {
		require.Equal(t, code, err.ErrorResponse.Error.Code, "unexpected error code")
	}
}

// ValidateSingleDetail reports success if the error code matches the expected code and the detail item is found in the error response..
func ValidateSingleDetail(code string, detail DeploymentErrorDetail) func(*testing.T, *radcli.CLIError) {
	return func(t *testing.T, err *radcli.CLIError) {
		require.Equal(t, code, err.ErrorResponse.Error.Code, "unexpected error code")
		for _, candidate := range err.ErrorResponse.Error.Details {
			if detail.Matches(candidate) {
				return
			}
		}

		require.Fail(t, "failed to find a matching error detail")
	}
}

// ValidateAnyDetails reports success if any of the provided error details are found in the error response.
func ValidateAnyDetails(code string, details []DeploymentErrorDetail) func(*testing.T, *radcli.CLIError) {
	return func(t *testing.T, err *radcli.CLIError) {
		require.Equal(t, code, err.ErrorResponse.Error.Code, "unexpected error code")
		for _, detail := range details {
			for _, candidate := range err.ErrorResponse.Error.Details {
				if detail.Matches(candidate) {
					return
				}
			}
		}

		require.Fail(t, "failed to find a matching error detail")
	}
}

// ValidateAllDetails reports success if all of the provided error details are found in the error response.
func ValidateAllDetails(code string, details []DeploymentErrorDetail) func(*testing.T, *radcli.CLIError) {
	return func(t *testing.T, err *radcli.CLIError) {
		require.Equal(t, code, err.ErrorResponse.Error.Code, "unexpected error code")
		for _, detail := range details {
			matched := false
			for _, candidate := range err.ErrorResponse.Error.Details {
				if detail.Matches(candidate) {
					matched = true
					break
				}
			}

			if !matched {
				assert.Failf(t, "failed to find a matching error detail with Code: %s and Message: %s", detail.Code, detail.MessageContains)
			}
		}
	}
}
