// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package create

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/configFile"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid create command",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command without workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithoutWorkspace,
			},
		},
		{
			Name:          "Create command with invalid environment",
			Input:         []string{"testingenv", "-e", "testingenv"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid namespace",
			Input:         []string{"-n", "invalidnamespace"}, // TODO
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid workspace",
			Input:         []string{"-w", "invalidworkspace"}, // TODO
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Create command with invalid resource group",
			Input:         []string{"-g", "invalidresourcegroup"}, // TODO
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	// TODO add failure cases
	t.Run("Run env create", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		k8sGoClient :=
			fake.NewSimpleClientset(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "default",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			})

		namespaceClient := namespace.NewMockInterface(ctrl)
		namespaceClient.EXPECT().
			ValidateNamespace(context.Background(), k8sGoClient, "default").
			Return(nil).Times(1)

		appManagementClient.EXPECT().
			CheckUCPGroupExistence(context.Background(), "radius", "local", "default").
			Return(true, nil).Times(1)

		appManagementClient.EXPECT().
			CreateEnvironment(context.Background(), "default", "global", "default", "Kubernetes", gomock.Any()).
			Return(true, nil).Times(1)

		configFileInterface := configFile.NewMockInterface(ctrl)
		configFileInterface.EXPECT().
			EditWorkspaces(context.Background(), "filePath", "defaultWorkspace", "default", "default").
			Return(nil).Times(1)

		outputSink := &output.MockOutput{}

		workspace := &workspaces.Workspace{
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": "kind-kind",
			},

			Name: "defaultWorkspace",
		}

		runner := &Runner{
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			ConfigHolder:        &framework.ConfigHolder{ConfigFilePath: "filePath"},
			Output:              outputSink,
			Workspace:           workspace,
			EnvironmentName:     "default",
			UCPResourceGroup:    "default",
			Namespace:           "default",
			K8sGoClient:         k8sGoClient,
			NamespaceInterface:  namespaceClient,
			ConfigFileInterface: configFileInterface,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}
