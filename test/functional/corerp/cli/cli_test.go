// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_CLI(t *testing.T) {
	template := "testdata/corerp-kubernetes-cli.bicep"
	name := "kubernetes-cli"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "kubernetes-cli",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "containera",
						Type:    validation.ContainersResource,
						AppName: "kubernetes-cli",
					},
					{
						Name:    "containerb",
						Type:    validation.ContainersResource,
						AppName: "kubernetes-cli",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "containera"),
						validation.NewK8sPodForResource(name, "containerb"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_CLI_DeploymentParameters(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	template := "testdata/corerp-kubernetes-cli-parameters.bicep"
	parameterFile := "testdata/corerp-kubernetes-cli-parameters.parameters.json"
	name := "kubernetes-cli-params"
	parameterFilePath := filepath.Join(cwd, parameterFile)

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{Executor: step.NewDeployExecutor(template, "@"+parameterFilePath),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "kubernetes-cli-params",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "containerc",
						Type:    validation.ContainersResource,
						AppName: "kubernetes-cli-params",
					},
					{
						Name:    "containerd",
						Type:    validation.ContainersResource,
						AppName: "kubernetes-cli-params",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "containerc"),
						validation.NewK8sPodForResource(name, "containerd"),
					},
				},
			},
		},
	})

	test.Test(t)
}
