// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli_test

import (
	"testing"

	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_AppDeploy_ScaffoldedApp(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	options := azuretest.NewTestOptions(t)

	// We deploy a simple app and then run a variety of different CLI commands on it. Emphasis here
	// is on the commands that aren't tested as part of our main flow.
	//
	// We use nested tests so we can skip them if we've already failed deployment.
	application := "azure-cli-scaffolded"

	tempDir := t.TempDir()
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	cli.WorkingDirectory = tempDir

	t.Logf("scaffolding %s with `rad app init`", application)
	err := cli.ApplicationInit(ctx, application)
	require.NoErrorf(t, err, "failed to run `rad app init` %s", application)
	t.Logf("done scaffolding %s with `rad app init`", application)

	t.Logf("deploying %s with `rad app deploy`", application)
	err = cli.ApplicationDeploy(ctx)
	require.NoErrorf(t, err, "failed to run `rad app deploy` %s", application)
	t.Logf("done deploying %s with `rad app deploy`", application)

	// Running for the side effect of making sure the pods are started.
	validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
		Namespaces: map[string][]validation.K8sObject{
			application: {
				validation.NewK8sPodForResource(application, "demo"),
			},
		},
	})
}
