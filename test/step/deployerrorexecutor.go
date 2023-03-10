// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

	err = cli.Deploy(ctx, templateFilePath, d.Parameters...)
	require.ErrorContains(t, err, "Failed", d.Description)

	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
