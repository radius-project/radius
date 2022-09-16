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
	// configWithoutWorkspace := radcli.LoadConfigWithoutWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid env create",
			Input:         []string{"-e", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete command with incorrect args",
			Input:         []string{""},
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
	t.Run("Validate environment created with valid inputs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		k8sGoClient :=
			fake.NewSimpleClientset(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "idk",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			})

		outputSink := &output.MockOutput{}

		runner := &Runner{
			Output: outputSink,
			Workspace: &workspaces.Workspace{
				Connection: map[string]interface{}{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name: "kind-kind",
			},
			EnvironmentName:  "prod",
			UCPResourceGroup: "default",
			Namespace:        "default",
			K8sGoClient:      k8sGoClient,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

	})
	t.Run("Validate Scenario 2", func(t *testing.T) {

	})
	t.Run("Validate Scenario 3", func(t *testing.T) {

	})
	t.Run("Validate Scenario i", func(t *testing.T) {

	})
}

// func newSimpleK8s() *k8s {
// 	client := k8s{}
// 	client.clientset = fake.NewSimpleClientset()
// 	return &client
// }
