// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"testing"

	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	testcases := []radcli.ValidateInput{
		{
			Input:         []string{"show", "containers", "foo"},
			ExpectedValid: true,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("show resource", func(t *testing.T) {
		t.Run("resource_found", func(t *testing.T) {
			factory := &radcli.BaseFactory{}

			// Set up mocks here

			args := []string{}
			cmd, runner := NewCommand(factory)
			err := radcli.RunCommand(t, args, cmd, runner)
			require.NoError(t, err)

			// Validate mocks/output here
		})
		t.Run("resource_not_found", func(t *testing.T) {

		})
		t.Run("unsupported_type", func(t *testing.T) {

		})
	})
}
