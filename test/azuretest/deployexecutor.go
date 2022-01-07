// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azuretest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

var _ StepExecutor = (*DeployStepExecutor)(nil)

type DeployStepExecutor struct {
	Description string
	Template    string
	Parameters  []string
}

func NewDeployStepExecutor(template string) *DeployStepExecutor {
	return &DeployStepExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
	}
}

func (d *DeployStepExecutor) GetDescription() string {
	return d.Description
}

func (d *DeployStepExecutor) Execute(ctx context.Context, t *testing.T, options TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath, d.Parameters...)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
