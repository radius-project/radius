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

var _ Executor = (*DeployExecutor)(nil)

type DeployExecutor struct {
	Description string
	Template    string
	Parameters  []string

	// Application sets the `--application` command-line parameter. This is needed in cases where
	// the application is not defined in bicep.
	Application string
}

func NewDeployExecutor(template string, parameters ...string) *DeployExecutor {
	return &DeployExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
	}
}

func (d *DeployExecutor) WithApplication(application string) *DeployExecutor {
	d.Application = application
	return d
}

func (d *DeployExecutor) GetDescription() string {
	return d.Description
}

func (d *DeployExecutor) Execute(ctx context.Context, t *testing.T, options test.TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath, d.Application, d.Parameters...)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}
