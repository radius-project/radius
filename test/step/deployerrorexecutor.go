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
}

func NewDeployErrorExecutor(template string, errCode string, parameters ...string) *DeployErrorExecutor {
	return &DeployErrorExecutor{
		Description:       fmt.Sprintf("deploy %s", template),
		Template:          template,
		Parameters:        parameters,
		ExpectedErrorCode: errCode,
	}
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

	err1 := cli.Deploy(ctx, templateFilePath, d.Parameters...)
	require.Error(t, err1, "deployment %s succeeded when it should have failed", d.Description)

	var cliErr *radcli.CLIError
	err = errors.Unwrap(err1)
	ok := errors.As(err, &cliErr)
	t.Logf("ok is %t", ok)
	require.True(t, ok)
	t.Logf("expected error code: %s but got %s", d.ExpectedErrorCode, cliErr.GetFirstErrorCode())
	require.Equal(t, d.ExpectedErrorCode, cliErr.GetFirstErrorCode())

	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
