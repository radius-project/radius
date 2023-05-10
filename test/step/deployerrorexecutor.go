// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package step

import (
	"context"
	"errors"
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
	Description       string
	Template          string
	Parameters        []string
	ExpectedErrorCode string

	// Application sets the `--application` command-line parameter. This is needed in cases where
	// the application is not defined in bicep.
	Application string
}

func NewDeployErrorExecutor(template string, errCode string, parameters ...string) *DeployErrorExecutor {
	return &DeployErrorExecutor{
		Description:       fmt.Sprintf("deploy %s", template),
		Template:          template,
		Parameters:        parameters,
		ExpectedErrorCode: errCode,
	}
}

func (d *DeployErrorExecutor) WithApplication(application string) *DeployErrorExecutor {
	d.Application = application
	return d
}

func (d *DeployErrorExecutor) GetDescription() string {
	return d.Description
}

func (d *DeployErrorExecutor) Execute(ctx context.Context, t *testing.T, options test.TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath, d.Application, d.Parameters...)
	require.Error(t, err, "deployment %s succeeded when it should have failed", d.Description)

	var cliErr *radcli.CLIError
	ok := errors.As(err, &cliErr)
	require.True(t, ok)
	require.Equal(t, d.ExpectedErrorCode, cliErr.GetFirstErrorCode())

	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
