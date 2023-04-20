// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package envswitch

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/framework"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Switch Command with valid arguments",
			Input:         []string{"validEnvToSwitchTo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createGetEnvDetailsSuccess(mocks.ApplicationManagementClient)
			},
		},
		{
			Name:          "Switch Command with non-existent env",
			Input:         []string{"nonExistentEnv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createGetEnvDetailsError(mocks.ApplicationManagementClient)
			},
		},
		{
			Name:          "Switch Command with non-editable workspace invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: radcli.LoadEmptyConfig(t)},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func createGetEnvDetailsSuccess(appManagementClient *clients.MockApplicationsManagementClient) {
	appManagementClient.EXPECT().
		GetEnvDetails(gomock.Any(), "validEnvToSwitchTo").
		Return(corerp.EnvironmentResource{}, nil).Times(1)
}

func createGetEnvDetailsError(appManagementClient *clients.MockApplicationsManagementClient) {
	responseError := &azcore.ResponseError{}
	responseError.ErrorCode = v1.CodeNotFound
	err := error(responseError)

	appManagementClient.EXPECT().
		GetEnvDetails(gomock.Any(), "nonExistentEnv").
		Return(corerp.EnvironmentResource{}, err).Times(1)
}
