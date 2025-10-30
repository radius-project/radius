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
package show

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "missing recipe pack name",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "invalid workspace reference",
			Input:         []string{"my-pack", "-w", "doesnotexist"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "valid with workspace flag",
			Input:         []string{"my-pack", "-w", radcli.TestWorkspaceName},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "valid with fallback workspace",
			Input:         []string{"my-pack", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	recipePack := corerpv20250801preview.RecipePackResource{
		Name: to.Ptr("sample-pack"),
		Properties: &corerpv20250801preview.RecipePackProperties{
			Recipes: map[string]*corerpv20250801preview.RecipeDefinition{
				"Radius.Core/example": {
					RecipeKind:     to.Ptr(corerpv20250801preview.RecipeKindTerraform),
					RecipeLocation: to.Ptr("https://github.com/radius-project/example"),
					Parameters: map[string]any{
						"foo": "bar",
					},
				},
			},
		},
	}

	appMgmtClient := clients.NewMockApplicationsManagementClient(ctrl)
	appMgmtClient.EXPECT().
		GetRecipePack(gomock.Any(), "sample-pack").
		Return(recipePack, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}

	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appMgmtClient},
		Workspace:         workspace,
		Output:            outputSink,
		RecipePackName:    "sample-pack",
		Format:            "json",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expected := []any{
		output.FormattedOutput{
			Format:  "json",
			Obj:     recipePack,
			Options: objectformats.GetRecipePackTableFormat(),
		},
	}

	require.Equal(t, expected, outputSink.Writes)
}
