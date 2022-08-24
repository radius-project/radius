// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package show

import (
	"testing"

	"github.com/project-radius/radius/pkg/cli/cmd/shared"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace()
	testcases := []radcli.ValidateInput{
		{
			Input:         []string{"containers", "foo", "-o", "table"},
			ExpectedValid: true,
			ConfigHolder:  shared.ConfigHolder{"", config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func initMocks() {

}
