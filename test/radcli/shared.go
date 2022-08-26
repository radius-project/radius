// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radcli

import (
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type ValidateInput struct {
	Input         []string
	ExpectedValid bool
	ConfigHolder  framework.ConfigHolder
}

func SharedCommandValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner)) {
	cmd, _ := factory(&framework.Impl{})
	require.NotNil(t, cmd.Args, "Args is required")
	require.NotEmpty(t, cmd.Example, "Example is required")
	require.NotEmpty(t, cmd.Long, "Long is required")
	require.NotEmpty(t, cmd.Short, "Short is required")
	require.NotEmpty(t, cmd.Use, "Use is required")
	require.NotNil(t, cmd.RunE, "RunE is required")
}

func SharedValidateValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner), testcases []ValidateInput) {
	for _, testcase := range testcases {
		t.Run(strings.Join(testcase.Input, " "), func(t *testing.T) {
			framework := &framework.Impl{
				ConnectionFactory: nil,
				ConfigHolder:      &testcase.ConfigHolder,
				Output:            nil}
			cmd, runner := factory(framework)
			cmd.SetArgs(testcase.Input)

			err := cmd.ParseFlags(testcase.Input)
			require.NoError(t, err, "flag parsing failed")

			err = runner.Validate(cmd, cmd.Flags().Args())
			if testcase.ExpectedValid {
				require.NoError(t, err, "validation should have passed but it failed")
			} else {
				require.Error(t, err, "validation should have failed but it passed")
			}
		})
	}
}

func LoadConfigWithWorkspace() *viper.Viper {
	v, err := cli.LoadConfig("./testdata/config.yaml")
	if err != nil {
		return nil
	}
	return v
}
