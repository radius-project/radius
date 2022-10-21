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
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	ctrl := gomock.NewController(t)
	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	testResourceGroup := v20220315privatepreview.ResourceGroupResource{}

	// Non-existing application
	createShowApplicationError(appManagementClient, testResourceGroup)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Switch Command with non-existing app",
			Input:         []string{"appToSwitchTo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func createShowApplicationError(appManagementClient *clients.MockApplicationsManagementClient, testResourceGroup v20220315privatepreview.ResourceGroupResource) {
	responseError := &azcore.ResponseError{}
	responseError.ErrorCode = v1.CodeNotFound
	err := error(responseError)

	appManagementClient.EXPECT().
		ShowApplication(gomock.Any(), "appToSwitchTo").
		Return(corerp.ApplicationResource{}, err).Times(1)
}
