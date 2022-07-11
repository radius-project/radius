// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_Environment(t *testing.T) {
	template := "testdata/corerp-resources-environment.bicep"
	name := "corerp-resources-environment"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-environment-env",
					Type: validation.EnvironmentsResource,
				},
			},
		},
	})

	test.Test(t)
}

func Test_EnvironmentParamInjection(t *testing.T) {

	ctx, _ := test.GetContext(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	params := map[string]map[string]interface{}{}

	err = environments.InjectEnvironmentParam(params, ctx, env)
	require.NoError(t, err, "failed to inject environment param")

	temp := params["environment"]["value"].(*string)
	// Only check the first part as we don't want this test to fail if the environment name changes
	require.Contains(t, *temp, "/planes/radius/local/resourcegroups")
}
