// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package appswitch

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/config"
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
			Name:          "Switch Command with valid command",
			Input:         []string{"validAppToSwitchTo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createShowApplicationSuccess(mocks.ApplicationManagementClient)
			},
		},
		{
			Name:          "Switch Command with non-editable workspace invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: radcli.LoadEmptyConfig(t)},
		},
		{
			Name:          "Switch Command with non-existent app",
			Input:         []string{"nonExistentApp"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createShowApplicationError(mocks.ApplicationManagementClient)
			},
		},
		{
			Name:          "Switch Command with flag",
			Input:         []string{"-a", "validAppToSwitchTo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createShowApplicationSuccess(mocks.ApplicationManagementClient)
			},
		},
		{
			// Test that DirectoryConfig does NOT affect this comment. 'app switch' only operates on config.yaml.
			Name:          "Switch Command requires flag or arg",
			Input:         []string{"-a", "validAppToSwitchTo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
				DirectoryConfig: &config.DirectoryConfig{
					Workspace: config.DirectoryWorkspaceConfig{
						Application: "somethingElse",
					},
				},
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				createShowApplicationSuccess(mocks.ApplicationManagementClient)
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func createShowApplicationSuccess(appManagementClient *clients.MockApplicationsManagementClient) {
	appManagementClient.EXPECT().
		ShowApplication(gomock.Any(), "validAppToSwitchTo").
		Return(corerp.ApplicationResource{}, nil).Times(1)
}

func createShowApplicationError(appManagementClient *clients.MockApplicationsManagementClient) {
	responseError := &azcore.ResponseError{}
	responseError.ErrorCode = v1.CodeNotFound
	err := error(responseError)

	appManagementClient.EXPECT().
		ShowApplication(gomock.Any(), "nonExistentApp").
		Return(corerp.ApplicationResource{}, err).Times(1)
}
